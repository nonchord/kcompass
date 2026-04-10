package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// clusterListResponse is the JSON envelope returned by an HTTP backend.
type clusterListResponse struct {
	Clusters []ClusterRecord `json:"clusters"`
}

// HTTPBackend fetches cluster records from an HTTP endpoint.
type HTTPBackend struct {
	name     string
	url      string
	tokenEnv string // name of env var holding the bearer token
	client   *http.Client
}

// HTTPBackendConfig holds all options for NewHTTPBackend.
type HTTPBackendConfig struct {
	Name     string
	URL      string
	TokenEnv string       // env var name for bearer token; empty means no auth
	Client   *http.Client // nil uses a default client with a 30s timeout
}

// NewHTTPBackend creates an HTTPBackend.
func NewHTTPBackend(cfg HTTPBackendConfig) *HTTPBackend {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPBackend{
		name:     cfg.Name,
		url:      cfg.URL,
		tokenEnv: cfg.TokenEnv,
		client:   client,
	}
}

// Name implements Backend.
func (b *HTTPBackend) Name() string { return b.name }

// List implements Backend.
func (b *HTTPBackend) List(ctx context.Context) ([]ClusterRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url, nil)
	if err != nil {
		return nil, fmt.Errorf("http backend: build request: %w", err)
	}
	if b.tokenEnv != "" {
		if token := os.Getenv(b.tokenEnv); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http backend: request to %s: %w", b.url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http backend: %s returned status %d", b.url, resp.StatusCode)
	}

	var result clusterListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("http backend: decode response: %w", err)
	}
	if result.Clusters == nil {
		return []ClusterRecord{}, nil
	}
	return result.Clusters, nil
}

// Get implements Backend.
func (b *HTTPBackend) Get(ctx context.Context, name string) (*ClusterRecord, error) {
	records, err := b.List(ctx)
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
