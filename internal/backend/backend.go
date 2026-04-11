// Package backend defines the Backend interface and shared types used by all backends.
package backend

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a cluster name doesn't exist in a backend.
var ErrNotFound = errors.New("cluster not found")

// ErrAccessDenied is returned by a backend when the underlying source rejects
// the caller's credentials. Backends wrap their native auth errors with this
// sentinel so the CLI can surface a consistent, friendly message regardless
// of whether the underlying transport was SSH, HTTPS, or something else.
var ErrAccessDenied = errors.New("access denied to cluster inventory")

// ClusterRecord is the canonical representation of a Kubernetes cluster.
type ClusterRecord struct {
	Name        string            `yaml:"name"                  json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"      json:"labels,omitempty"`
	Kubeconfig  KubeconfigSpec    `yaml:"kubeconfig"            json:"kubeconfig"`
}

// KubeconfigSpec describes how to obtain a kubeconfig fragment for a cluster.
// Exactly one of Inline or Command must be set.
type KubeconfigSpec struct {
	// Inline is a complete kubeconfig YAML blob shipped with the record.
	// Use this when the same kubeconfig works for everyone (OIDC with kubelogin,
	// kube-oidc-proxy, service account tokens behind Netbird, etc).
	Inline string `yaml:"inline,omitempty" json:"inline,omitempty"`

	// Command is an argv vector. kcompass runs it with KUBECONFIG set to a
	// fresh temp file, then merges that file into the user's kubeconfig.
	// Use this for tools that mint per-user credentials (tailscale, gcloud, aws).
	Command []string `yaml:"command,omitempty" json:"command,omitempty"`
}

// Validate reports whether exactly one of Inline or Command is set.
func (k KubeconfigSpec) Validate() error {
	hasInline := k.Inline != ""
	hasCommand := len(k.Command) > 0
	switch {
	case !hasInline && !hasCommand:
		return errors.New("kubeconfig: must specify either inline or command")
	case hasInline && hasCommand:
		return errors.New("kubeconfig: inline and command are mutually exclusive")
	}
	return nil
}

// Validate reports whether the record is structurally valid for use by connect.
func (r ClusterRecord) Validate() error {
	if r.Name == "" {
		return errors.New("cluster: name is required")
	}
	if err := r.Kubeconfig.Validate(); err != nil {
		return err
	}
	return nil
}

// Backend is the interface all cluster sources must implement.
type Backend interface {
	// Name returns the unique identifier for this backend instance.
	Name() string
	// List returns all cluster records visible to this backend.
	List(ctx context.Context) ([]ClusterRecord, error)
	// Get returns a single cluster record by name, or ErrNotFound.
	Get(ctx context.Context, name string) (*ClusterRecord, error)
}
