package discovery_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/discovery"
)

func TestGCloudProbeReturnsNil(t *testing.T) {
	b, err := discovery.GCloudProbe()(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b, "GCloudProbe is a stub until GKE backend is implemented")
}

func TestAWSProbeReturnsNil(t *testing.T) {
	b, err := discovery.AWSProbe()(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b, "AWSProbe is a stub until EKS backend is implemented")
}

func TestDefaultProbesLength(t *testing.T) {
	probes := discovery.DefaultProbes()
	require.NotEmpty(t, probes)
	// Tailscale, Netbird, DNS, GCloud, AWS
	assert.Len(t, probes, 5)
}

func TestDefaultProbesAllReturnNilInCI(t *testing.T) {
	// This test verifies that in a pristine environment (CI with no Tailscale,
	// Netbird, DNS search domains, or cloud credentials) all default probes
	// return (nil, nil) gracefully. DefaultProbes() reads real system state, so
	// on any dev machine attached to a real network with a published kcompass
	// TXT record it will legitimately return a backend — skip there.
	if os.Getenv("CI") == "" {
		t.Skip("DefaultProbes() reads real system state; test only valid in CI")
	}
	probes := discovery.DefaultProbes()
	for i, probe := range probes {
		b, err := probe(context.Background())
		assert.NoError(t, err, "probe %d returned unexpected error", i)
		assert.Nil(t, b, "probe %d returned unexpected backend in CI", i)
	}
}
