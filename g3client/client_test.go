package g3client

import (
	"net/http"
	"testing"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/hashicorp/go-retryablehttp"
)

func TestGen3ClientInitializesSyfonClient(t *testing.T) {
	logger, cleanup := logs.New("g3client-test", logs.WithNoConsole(), logs.WithNoMessageFile())
	t.Cleanup(cleanup)

	req := &request.Request{
		RetryClient: &retryablehttp.Client{HTTPClient: &http.Client{}},
	}
	cred := &conf.Credential{
		Profile:     "test",
		APIEndpoint: "https://example.org",
	}
	client := &Gen3Client{
		RequestInterface: req,
		credential:       cred,
		logger:           logger,
		requestedClients: []ClientType{SyfonClient},
	}

	client.initializeClients()

	syfon := client.SyfonClient()
	if syfon == nil {
		t.Fatal("expected syfon client to be initialized")
	}
	if syfon.Health() == nil || syfon.Data() == nil || syfon.Index() == nil || syfon.DRS() == nil || syfon.Buckets() == nil || syfon.Metrics() == nil || syfon.LFS() == nil {
		t.Fatal("expected syfon services to be initialized")
	}

	if client.FenceClient() != nil {
		t.Fatal("did not expect fence client to be initialized when it was not requested")
	}
	if client.SowerClient() != nil {
		t.Fatal("did not expect sower client to be initialized when it was not requested")
	}
	if client.RequestorClient() != nil {
		t.Fatal("did not expect requestor client to be initialized when it was not requested")
	}

	if got := client.Credentials(); got == nil {
		t.Fatal("expected credentials manager")
	} else if current := got.Current(); current != cred {
		t.Fatal("expected credentials manager to return original credential")
	}
}
