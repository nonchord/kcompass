package discovery

import (
	"context"

	"github.com/nonchord/kcompass/internal/backend"
)

// GCloudProbe returns a ProbeFunc that would detect gcloud credentials and
// construct a GKE backend. Not yet implemented (GKE backend is phase 3).
func GCloudProbe() ProbeFunc {
	return func(_ context.Context) (backend.Backend, error) {
		return nil, nil
	}
}

// AWSProbe returns a ProbeFunc that would detect AWS credentials and
// construct an EKS backend. Not yet implemented (EKS backend is phase 4).
func AWSProbe() ProbeFunc {
	return func(_ context.Context) (backend.Backend, error) {
		return nil, nil
	}
}
