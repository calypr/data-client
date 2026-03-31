package gen3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/request"
)

type testRequestClient struct {
	client *http.Client
}

func (t *testRequestClient) New(method, url string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: url, Headers: map[string]string{}}
}

func (t *testRequestClient) Do(ctx context.Context, rb *request.RequestBuilder) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, rb.Method, rb.Url, rb.Body)
	if err != nil {
		return nil, err
	}
	for k, v := range rb.Headers {
		req.Header.Set(k, v)
	}
	return t.client.Do(req)
}

func TestResolveUploadURLsUsesSingleBulkRequest(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/data/upload/bulk" {
			http.NotFound(w, r)
			return
		}
		calls++
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"file_id": "did-1", "file_name": "one.bin", "url": "https://signed/one", "status": 200},
				{"file_id": "did-2", "file_name": "two.bin", "url": "https://signed/two", "status": 200},
			},
		})
	}))
	defer srv.Close()

	signer := New(
		&testRequestClient{client: srv.Client()},
		&conf.Credential{APIEndpoint: srv.URL},
		nil,
		nil,
	)

	out, err := signer.ResolveUploadURLs(context.Background(), []common.UploadURLResolveRequest{
		{GUID: "did-1", Filename: "one.bin", Bucket: "b1"},
		{GUID: "did-2", Filename: "two.bin", Bucket: "b1"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadURLs error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one bulk call, got %d", calls)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(out))
	}
	if out[0].URL == "" || out[1].URL == "" {
		t.Fatalf("expected signed URLs in both results, got %+v", out)
	}
}
