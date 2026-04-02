package local

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/drs"
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

func TestResolveUploadURLsBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/data/upload/bulk" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		results := map[string]any{
			"results": []map[string]any{
				{"file_id": "did-1", "file_name": "one.bin", "url": "https://signed/one", "status": 200},
				{"file_id": "did-2", "file_name": "two.bin", "status": 400, "error": "bucket credential not found"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	req := &testRequestClient{client: srv.Client()}
	dc := drs.NewLocalDrsClient(req, srv.URL, logger)
	signer := New(srv.URL, nil, dc)

	out, err := signer.ResolveUploadURLs(context.Background(), []common.UploadURLResolveRequest{
		{GUID: "did-1", Filename: "one.bin", Bucket: "b1"},
		{GUID: "did-2", Filename: "two.bin", Bucket: "b1"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadURLs returned error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	if out[0].Status != http.StatusOK || out[0].URL == "" {
		t.Fatalf("expected first result success, got %+v", out[0])
	}
	if out[1].Status != http.StatusBadRequest || !strings.Contains(out[1].Error, "bucket credential not found") {
		t.Fatalf("expected second result error, got %+v", out[1])
	}
}
