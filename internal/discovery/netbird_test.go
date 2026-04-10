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
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.app.netbird.io": {"v=kc1; backend=https://github.com/company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "github.com")
}

func TestNetbirdProbeInterfaceAbsent(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return false },
		RunStatus:       netbirdStatusErr(errors.New("not running")),
		LookupTXT:       mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeStatusFails(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatusErr(errors.New("netbird not installed")),
		LookupTXT:       mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeInvalidJSON(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       func(_ context.Context) ([]byte, error) { return []byte("bad json"), nil },
		LookupTXT:       mockTXT(map[string][]string{}),
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
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeNoTXTRecord(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com:443"),
		LookupTXT:       mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeTXTError(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com:443"),
		LookupTXT: func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("NXDOMAIN")
		},
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeURLWithoutPort(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.mgmt.company.com": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "github.com")
}

func TestNetbirdProbeDefaultInterfaceAbsent(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return false },
		LookupTXT:       mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestNetbirdProbeHTTPScheme(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("http://mgmt.internal:80"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.mgmt.internal": {"v=kc1; backend=https://github.com/company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "github.com")
}

func TestNetbirdProbeWrongTXTFormat(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://mgmt.company.com:443"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.mgmt.company.com": {"v=spf1 include:example.com ~all"},
		}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}
