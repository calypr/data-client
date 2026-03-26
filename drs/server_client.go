package drs

import "github.com/calypr/data-client/transfer"

// ServerClient composes DRS metadata operations and transfer operations
// against the same server endpoint/runtime mode.
type ServerClient interface {
	Client
	transfer.Backend
}

type composedServerClient struct {
	Client
	transfer.Backend
}

func ComposeServerClient(c Client, b transfer.Backend) ServerClient {
	return &composedServerClient{
		Client:  c,
		Backend: b,
	}
}

