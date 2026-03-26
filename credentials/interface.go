package credentials

import (
	"context"

	"github.com/calypr/data-client/conf"
)

// Reader exposes current in-memory credential state.
type Reader interface {
	Current() *conf.Credential
}

// Manager exposes read and export operations for credentials.
type Manager interface {
	Reader
	Export(ctx context.Context, cred *conf.Credential) error
}
