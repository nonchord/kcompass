// Package backend defines the Backend interface and shared types used by all backends.
package backend

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a cluster name doesn't exist in a backend.
var ErrNotFound = errors.New("cluster not found")

// ClusterRecord is the canonical representation of a Kubernetes cluster.
type ClusterRecord struct {
	Name        string            `yaml:"name"        json:"name"`
	Description string            `yaml:"description" json:"description"`
	Provider    string            `yaml:"provider"    json:"provider"`
	Auth        string            `yaml:"auth"        json:"auth"`
	Metadata    map[string]string `yaml:"metadata"    json:"metadata"`
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
