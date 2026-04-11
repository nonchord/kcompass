// Package kubeconfig handles merging cluster credentials into the user's kubeconfig.
package kubeconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// MergeStatic merges a static kubeconfig blob (from a ClusterRecord's metadata
// "kubeconfig" field) into the kubeconfig at kubeconfigPath.  The file is written
// atomically.  If switchContext is true the current-context is updated to the
// merged cluster's context name (suffixed if a collision was resolved).
func MergeStatic(kubeconfigPath string, newKubeconfigData []byte, switchContext bool) (string, error) {
	existing, err := loadOrEmpty(kubeconfigPath)
	if err != nil {
		return "", err
	}

	incoming, err := clientcmd.Load(newKubeconfigData)
	if err != nil {
		return "", fmt.Errorf("kubeconfig: parse incoming: %w", err)
	}

	finalContext, err := mergeInto(existing, incoming)
	if err != nil {
		return "", err
	}

	if switchContext {
		existing.CurrentContext = finalContext
	}

	if err := writeAtomic(kubeconfigPath, existing); err != nil {
		return "", err
	}
	return finalContext, nil
}

// loadOrEmpty reads an existing kubeconfig file, or returns an empty Config if
// the file does not exist.
func loadOrEmpty(path string) (*clientcmdapi.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := clientcmdapi.NewConfig()
		return cfg, nil
	}
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: load %s: %w", path, err)
	}
	return cfg, nil
}

// mergeInto copies all clusters, users, and contexts from src into dst.
// Collision resolution is content-aware: if src has an entry with the same
// name AND the same content as one already in dst, the existing slot is
// reused (so re-running `kcompass connect` against the same cluster is a
// no-op, not a source of -1, -2, … duplicates). If the content differs, a
// numeric suffix is appended, and context cluster/user refs are rewritten
// to point at the renamed targets. Returns the final context name the
// caller should switch to.
//
// When a slot is reused (content matched), the destination entry is left
// in place rather than overwritten. For contexts this is load-bearing: it
// preserves the user's per-context namespace preference set via
// `kubectl config set-context --current --namespace=foo` across repeated
// connects. For clusters and users it's a small optimization — the content
// was already equal so either behavior is semantically equivalent.
func mergeInto(dst, src *clientcmdapi.Config) (string, error) {
	// Determine the incoming context name (use the first one if multiple).
	var srcContext string
	for name := range src.Contexts {
		srcContext = name
		break
	}

	// Pass 1: merge clusters, tracking any renames so the context pass can
	// rewrite its cluster ref when a collision forced a suffix.
	clusterRename := map[string]string{}
	for name, cluster := range src.Clusters {
		final, reuse := resolveMergeName(dst.Clusters, name, func(existing *clientcmdapi.Cluster) bool {
			return equalCluster(existing, cluster)
		})
		if !reuse {
			dst.Clusters[final] = cluster
		}
		clusterRename[name] = final
	}

	// Pass 2: merge authinfos with the same rename-tracking.
	userRename := map[string]string{}
	for name, user := range src.AuthInfos {
		final, reuse := resolveMergeName(dst.AuthInfos, name, func(existing *clientcmdapi.AuthInfo) bool {
			return equalAuthInfo(existing, user)
		})
		if !reuse {
			dst.AuthInfos[final] = user
		}
		userRename[name] = final
	}

	// Pass 3: merge contexts. Each context may reference a cluster/user by
	// name; if the cluster/user got renamed in passes 1 or 2, rewrite the
	// ref on a copy so the stored context points at the right targets.
	// Then compare the rewritten value against the dst slot for content
	// equality (ignoring namespace — see equalContext), so re-running
	// connect is idempotent and does not overwrite a user's namespace.
	finalContext := srcContext
	for name, ctx := range src.Contexts {
		rewritten := *ctx // copy the struct so we don't mutate src
		if renamed, ok := clusterRename[ctx.Cluster]; ok {
			rewritten.Cluster = renamed
		}
		if renamed, ok := userRename[ctx.AuthInfo]; ok {
			rewritten.AuthInfo = renamed
		}
		final, reuse := resolveMergeName(dst.Contexts, name, func(existing *clientcmdapi.Context) bool {
			return equalContext(existing, &rewritten)
		})
		if name == srcContext {
			finalContext = final
		}
		if !reuse {
			dst.Contexts[final] = &rewritten
		}
	}

	return finalContext, nil
}

// resolveMergeName returns the name under which a new entry should be
// stored in m, along with a flag indicating whether the chosen slot
// already holds content-equivalent data. Three outcomes:
//
//  1. m has no entry at name → returns (name, false) — write to a new slot
//  2. m has an entry at name and equals(existing) reports true → returns
//     (name, true) — reuse the slot, do NOT overwrite
//  3. m has an entry at name with different content → tries name-1, name-2,
//     … until it finds either a free slot (returns (candidate, false)) OR
//     one that equals reports true (returns (candidate, true))
//
// Case (2) is what makes `kcompass connect` idempotent when re-run against
// the same cluster: the second call reuses the slot the first call wrote,
// instead of producing a fresh name-1 suffix every time. The reuse flag
// lets callers skip the write entirely, which for contexts is critical
// because it preserves the user's namespace preference.
func resolveMergeName[V any](m map[string]V, name string, equals func(V) bool) (string, bool) {
	if existing, exists := m[name]; !exists {
		return name, false
	} else if equals(existing) {
		return name, true
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		existing, exists := m[candidate]
		if !exists {
			return candidate, false
		}
		if equals(existing) {
			return candidate, true
		}
	}
}

// equalCluster, equalAuthInfo, and equalContext compare two kubeconfig
// entries by marshaling each to canonical JSON and comparing the bytes.
// json.Marshal is deterministic for plain Go structs: struct fields are
// emitted in declaration order, and map keys are sorted (per encoding/json
// doc). The clientcmdapi types have no custom MarshalJSON and no fields
// that hold pointers to shared mutable state, so two structurally-equivalent
// values always produce identical bytes.
//
// Using json.Marshal directly (rather than routing through clientcmd.Write
// + a Config wrapper) is simpler and keeps the comparison close to the
// domain: equality is defined by "would these two values serialize the
// same way?" LocationOfOrigin is excluded automatically via its `json:"-"`
// tag on all three types.
func equalCluster(a, b *clientcmdapi.Cluster) bool {
	if a == nil || b == nil {
		return a == b
	}
	return bytes.Equal(marshalJSON(a), marshalJSON(b))
}

func equalAuthInfo(a, b *clientcmdapi.AuthInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return bytes.Equal(marshalJSON(a), marshalJSON(b))
}

// equalContext compares two contexts ignoring the Namespace field. The
// namespace under a given context is a user-level preference (managed via
// `kubectl config set-context --current --namespace=foo`), not part of
// the identity of the cluster connection. Treating a namespace change as
// "different content" would defeat idempotency: every `kcompass connect`
// after a manual namespace switch would either rename to `-1` or clobber
// the user's choice. Ignoring it here, combined with the reuse flag in
// resolveMergeName, preserves the user's namespace on re-merge.
func equalContext(a, b *clientcmdapi.Context) bool {
	if a == nil || b == nil {
		return a == b
	}
	ac, bc := *a, *b
	ac.Namespace = ""
	bc.Namespace = ""
	return bytes.Equal(marshalJSON(&ac), marshalJSON(&bc))
}

// marshalJSON returns the canonical json.Marshal output for v. On marshal
// failure (which cannot happen for any field layout reachable here, since
// clientcmdapi types are all JSON-tagged plain structs) the function
// returns nil, which causes the comparison to treat the pair as unequal
// and fall through to the rename branch — a safe, conservative default.
func marshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// writeAtomic serialises cfg to a temp file in the same directory as path,
// then renames it into place.
func writeAtomic(path string, cfg *clientcmdapi.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("kubeconfig: create dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".kubeconfig-tmp-*")
	if err != nil {
		return fmt.Errorf("kubeconfig: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("kubeconfig: close temp file: %w", err)
	}

	if err := clientcmd.WriteToFile(*cfg, tmpName); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("kubeconfig: write: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("kubeconfig: rename into place: %w", err)
	}
	return nil
}
