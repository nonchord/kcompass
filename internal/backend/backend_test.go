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

func TestClusterRecordYAMLRoundtripCommand(t *testing.T) {
	original := backend.ClusterRecord{
		Name:        "nonchord-staging",
		Description: "Staging cluster",
		Labels:      map[string]string{"env": "staging", "team": "platform"},
		Kubeconfig: backend.KubeconfigSpec{
			Command: []string{"tailscale", "configure", "kubeconfig", "nonchord-staging"},
		},
	}
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded backend.ClusterRecord
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)
}

func TestClusterRecordYAMLRoundtripInline(t *testing.T) {
	original := backend.ClusterRecord{
		Name:        "dev",
		Description: "Local dev cluster",
		Kubeconfig: backend.KubeconfigSpec{
			Inline: "apiVersion: v1\nkind: Config\n",
		},
	}
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded backend.ClusterRecord
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)
}

func TestKubeconfigSpecValidate(t *testing.T) {
	cases := []struct {
		name    string
		spec    backend.KubeconfigSpec
		wantErr bool
	}{
		{"command only", backend.KubeconfigSpec{Command: []string{"echo", "hi"}}, false},
		{"inline only", backend.KubeconfigSpec{Inline: "apiVersion: v1"}, false},
		{"both set", backend.KubeconfigSpec{Inline: "x", Command: []string{"y"}}, true},
		{"neither set", backend.KubeconfigSpec{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClusterRecordValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rec := backend.ClusterRecord{
			Name:       "x",
			Kubeconfig: backend.KubeconfigSpec{Command: []string{"echo"}},
		}
		assert.NoError(t, rec.Validate())
	})
	t.Run("missing name", func(t *testing.T) {
		rec := backend.ClusterRecord{
			Kubeconfig: backend.KubeconfigSpec{Command: []string{"echo"}},
		}
		assert.Error(t, rec.Validate())
	})
	t.Run("invalid kubeconfig spec", func(t *testing.T) {
		rec := backend.ClusterRecord{Name: "x"}
		assert.Error(t, rec.Validate())
	})
}

func TestErrNotFound(t *testing.T) {
	assert.True(t, errors.Is(backend.ErrNotFound, backend.ErrNotFound))
	wrapped := fmt.Errorf("backend: %w", backend.ErrNotFound)
	assert.True(t, errors.Is(wrapped, backend.ErrNotFound))
}
