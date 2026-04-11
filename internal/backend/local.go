package backend

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// clusterFile is the on-disk format shared by the local and git backends.
type clusterFile struct {
	Clusters []ClusterRecord `yaml:"clusters"`
}

// LocalBackend reads cluster records from a local YAML file.
type LocalBackend struct {
	name string
	path string
}

// NewLocalBackend creates a LocalBackend. path may use ~ for the home directory.
func NewLocalBackend(name, path string) (*LocalBackend, error) {
	expanded, err := expandPath(path)
	if err != nil {
		return nil, fmt.Errorf("local backend: expand path: %w", err)
	}
	return &LocalBackend{name: name, path: expanded}, nil
}

// Name implements Backend.
func (b *LocalBackend) Name() string { return b.name }

// List implements Backend.
func (b *LocalBackend) List(_ context.Context) ([]ClusterRecord, error) {
	return readClusterFile(b.path)
}

// Get implements Backend.
func (b *LocalBackend) Get(_ context.Context, name string) (*ClusterRecord, error) {
	records, err := b.List(context.Background())
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].Name == name {
			return &records[i], nil
		}
	}
	return nil, ErrNotFound
}

// readClusterFile parses a YAML file in the clusters-list format and validates
// each record. A single invalid record fails the whole file with a clear error,
// so operators learn about broken inventory at parse time rather than connect time.
func readClusterFile(path string) ([]ClusterRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("local backend: read %s: %w", path, err)
	}
	var cf clusterFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("local backend: parse %s: %w", path, err)
	}
	if cf.Clusters == nil {
		return []ClusterRecord{}, nil
	}
	for i, rec := range cf.Clusters {
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("local backend: %s: cluster #%d (%q): %w", path, i, rec.Name, err)
		}
	}
	return cf.Clusters, nil
}
