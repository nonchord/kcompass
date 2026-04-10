package backend_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// clusterListResponse mirrors the server-side envelope for test servers.
type clusterListResponse struct {
	Clusters []backend.ClusterRecord `json:"clusters"`
}

func serveJSON(t *testing.T, records []backend.ClusterRecord) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(clusterListResponse{Clusters: records}))
	}))
}

func httpBackend(t *testing.T, srv *httptest.Server, tokenEnv string) *backend.HTTPBackend {
	t.Helper()
	return backend.NewHTTPBackend(backend.HTTPBackendConfig{
		Name:     "test-http",
		URL:      srv.URL,
		TokenEnv: tokenEnv,
		Client:   srv.Client(),
	})
}

var httpFixture = []backend.ClusterRecord{
	{Name: "http-cluster1", Description: "First.", Provider: "gke", Auth: "gcloud"},
	{Name: "http-cluster2", Description: "Second.", Provider: "eks", Auth: "aws"},
}

func TestHTTPBackendList(t *testing.T) {
	srv := serveJSON(t, httpFixture)
	defer srv.Close()

	b := httpBackend(t, srv, "")
	records, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "http-cluster1", records[0].Name)
	assert.Equal(t, "http-cluster2", records[1].Name)
}

func TestHTTPBackendListAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(clusterListResponse{Clusters: httpFixture}))
	}))
	defer srv.Close()

	t.Setenv("TEST_HTTP_TOKEN", "secret-token")
	b := backend.NewHTTPBackend(backend.HTTPBackendConfig{
		Name:     "test-http",
		URL:      srv.URL,
		TokenEnv: "TEST_HTTP_TOKEN",
		Client:   srv.Client(),
	})
	_, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", gotAuth)
}

func TestHTTPBackendListNoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(clusterListResponse{Clusters: httpFixture}))
	}))
	defer srv.Close()

	b := httpBackend(t, srv, "")
	_, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gotAuth)
}

func TestHTTPBackendListTokenEnvUnset(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(clusterListResponse{Clusters: httpFixture}))
	}))
	defer srv.Close()

	// tokenEnv is set in config but the env var itself is absent.
	b := backend.NewHTTPBackend(backend.HTTPBackendConfig{
		Name:     "test-http",
		URL:      srv.URL,
		TokenEnv: "KCOMPASS_TOKEN_THAT_DOES_NOT_EXIST",
		Client:   srv.Client(),
	})
	_, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gotAuth, "should not send Authorization when env var is unset")
}

func TestHTTPBackendGet(t *testing.T) {
	srv := serveJSON(t, httpFixture)
	defer srv.Close()

	b := httpBackend(t, srv, "")
	rec, err := b.Get(context.Background(), "http-cluster2")
	require.NoError(t, err)
	assert.Equal(t, "http-cluster2", rec.Name)
}

func TestHTTPBackendGetNotFound(t *testing.T) {
	srv := serveJSON(t, httpFixture)
	defer srv.Close()

	b := httpBackend(t, srv, "")
	_, err := b.Get(context.Background(), "nope")
	assert.True(t, errors.Is(err, backend.ErrNotFound))
}

func TestHTTPBackendServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := httpBackend(t, srv, "")
	_, err := b.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPBackendMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	b := httpBackend(t, srv, "")
	_, err := b.List(context.Background())
	require.Error(t, err)
}

func TestHTTPBackendEmptyClustersList(t *testing.T) {
	srv := serveJSON(t, nil)
	defer srv.Close()

	b := httpBackend(t, srv, "")
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestHTTPBackendContextCancelled(t *testing.T) {
	// Server delays long enough that a cancelled context beats it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	b := httpBackend(t, srv, "")
	_, err := b.List(ctx)
	require.Error(t, err)
}

func TestHTTPBackendTokenReadPerRequest(t *testing.T) {
	// Verify that the token is read from the env var on each request, not
	// captured at construction time.
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(clusterListResponse{Clusters: httpFixture}))
	}))
	defer srv.Close()

	t.Setenv("KCOMPASS_ROTATING_TOKEN", "token-v1")
	b := backend.NewHTTPBackend(backend.HTTPBackendConfig{
		Name:     "test-http",
		URL:      srv.URL,
		TokenEnv: "KCOMPASS_ROTATING_TOKEN",
		Client:   srv.Client(),
	})

	_, err := b.List(context.Background())
	require.NoError(t, err)

	t.Setenv("KCOMPASS_ROTATING_TOKEN", "token-v2")
	_, err = b.List(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer token-v1", "Bearer token-v2"}, tokens)
}
