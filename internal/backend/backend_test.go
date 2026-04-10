package backend_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/internal/backend"
)

func TestClusterRecordYAML(t *testing.T) {
	original := backend.ClusterRecord{
		Name:        "cluster1",
		Description: "The production cluster.",
		Provider:    "gke",
		Auth:        "gcloud",
		Metadata: map[string]string{
			"project":    "my-project",
			"region":     "us-east1",
			"cluster_id": "cluster1",
		},
	}
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded backend.ClusterRecord
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)
}

func TestErrNotFound(t *testing.T) {
	assert.True(t, errors.Is(backend.ErrNotFound, backend.ErrNotFound))
	wrapped := fmt.Errorf("backend: %w", backend.ErrNotFound)
	assert.True(t, errors.Is(wrapped, backend.ErrNotFound))
}
