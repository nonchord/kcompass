package discovery_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/discovery"
)

func netbirdStatus(mgmtURL string) func(ctx context.Context) ([]byte, error) {
	return func(_ context.Context) ([]byte, error) {
		return []byte(`{"managementState":{"URL":"` + mgmtURL + `"}}`), nil
	}
}

func netbirdStatusErr(err error) func(ctx context.Context) ([]byte, error) {
	return func(_ context.Context) ([]byte, error) { return nil, err }
}

func TestNetbirdProbeFound(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://app.netbird.io:443"),
		LookupSRV:       mockSRV("kcompass.app.netbird.io.", 8443),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "app.netbird.io")
}

func TestNetbirdProbeInterfaceAbsent(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return false },
		RunStatus:       netbirdStatusErr(errors.New("not running")),
		LookupSRV:       mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeStatusFails(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatusErr(errors.New("netbird not installed")),
		LookupSRV:       mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeInvalidJSON(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       func(_ context.Context) ([]byte, error) { return []byte("bad json"), nil },
		LookupSRV:       mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeEmptyURL(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus: func(_ context.Context) ([]byte, error) {
			return []byte(`{"managementState":{"URL":""}}`), nil
		},
		LookupSRV: mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeNoSRVRecord(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com:443"),
		LookupSRV:       mockSRVEmpty(),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeSRVError(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com:443"),
		LookupSRV:       mockSRVErr(errors.New("NXDOMAIN")),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

// TestNetbirdProbeDefaultInterfaceAbsent exercises the default RunStatus path
// where the wt0 interface is absent — it should return nil without forking a subprocess.
func TestNetbirdProbeDefaultInterfaceAbsent(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return false },
		// RunStatus is intentionally nil to use the default implementation.
		LookupSRV: mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeHTTPScheme(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("http://mgmt.internal:80"),
		LookupSRV:       mockSRV("kcompass.mgmt.internal.", 443),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "mgmt.internal")
}

func TestNetbirdProbeURLWithoutPort(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com"),
		LookupSRV:       mockSRV("kcompass.mgmt.company.com.", 443),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "mgmt.company.com")
}
