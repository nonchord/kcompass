package discovery_test

import (
	"context"
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
	// In a CI environment without Tailscale/Netbird/DNS/cloud credentials,
	// all default probes should return (nil, nil) gracefully.
	probes := discovery.DefaultProbes()
	for i, probe := range probes {
		b, err := probe(context.Background())
		assert.NoError(t, err, "probe %d returned unexpected error", i)
		assert.Nil(t, b, "probe %d returned unexpected backend in CI", i)
	}
}
