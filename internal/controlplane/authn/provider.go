package authn

import (
	"time"

	"github.com/fqix/kube-loop/internal/controlplane/storage"
)

// AccessIdentity is reconstructed from an opaque OAuth access token.
type AccessIdentity struct {
	Identity        storage.Identity
	ProviderID      string
	Groups          []string
	AuthorizationID string
	DeviceID        string
	TokenID         string
	AccessExpiresAt time.Time
}
