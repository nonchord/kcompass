// Package kubeconfig handles merging cluster credentials into the user's kubeconfig.
package kubeconfig

import (
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

// mergeInto copies all clusters, users, and contexts from src into dst,
// renaming any collisions with a -1, -2, ... suffix.  Returns the final
// context name that the caller should switch to (the context from src,
// possibly renamed).
func mergeInto(dst, src *clientcmdapi.Config) (string, error) {
	// Determine the incoming context name (use the first one if multiple).
	var srcContext string
	for name := range src.Contexts {
		srcContext = name
		break
	}

	for name, cluster := range src.Clusters {
		dst.Clusters[uniqueName(dst.Clusters, name)] = cluster
	}
	for name, user := range src.AuthInfos {
		dst.AuthInfos[uniqueName(dst.AuthInfos, name)] = user
	}

	finalContext := srcContext
	for name, ctx := range src.Contexts {
		final := uniqueName(dst.Contexts, name)
		if name == srcContext {
			finalContext = final
		}
		dst.Contexts[final] = ctx
	}

	return finalContext, nil
}

// uniqueName returns name if it is not a key in m, otherwise appends -1, -2, …
// until it finds one that is free.
func uniqueName[V any](m map[string]V, name string) string {
	if _, exists := m[name]; !exists {
		return name
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, exists := m[candidate]; !exists {
			return candidate
		}
	}
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
