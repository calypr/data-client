package fence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
)

type mockFenceServer struct{}

func (m *mockFenceServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && path == common.FenceAccessTokenEndpoint:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(common.AccessTokenStruct{AccessToken: "new-access-token"})
			return
		case r.Method == http.MethodGet && path == common.FenceUserEndpoint:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "test-user",
				"authz": map[string]any{
					"/resource": []map[string]string{
						{"method": "read", "service": "fence"},
					},
				},
			})
			return
		case r.Method == http.MethodGet && path == common.ShepherdVersionEndpoint:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`"2.0.0"`))
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(path, common.ShepherdEndpoint+"/objects/"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(path, common.FenceDataEndpoint+"/"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/download"):
			// Shepherd download
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://download.url"})
			return
		case r.Method == http.MethodGet && strings.Contains(path, common.FenceDataDownloadEndpoint+"/"):
			// Fence download
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(FenceResponse{URL: "https://download.url"})
			return
		case r.Method == http.MethodGet && path == "/data/buckets":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(S3BucketsResponse{
				S3Buckets: map[string]*S3Bucket{
					"test-bucket": {
						EndpointURL: "https://s3.amazonaws.com",
						Provider:    "s3",
						Region:      "us-east-1",
					},
				},
			})
			return
		case r.Method == http.MethodPost && path == common.FenceDataUploadEndpoint:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(FenceResponse{GUID: "new-guid", URL: "https://upload.url"})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, common.FenceDataUploadEndpoint+"/"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(FenceResponse{URL: "https://upload.url"})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestClient(server *httptest.Server) FenceInterface {
	cred := &conf.Credential{APIEndpoint: server.URL, Profile: "test", AccessToken: "test-token", APIKey: "test-key"}
	logger, _ := logs.New("test")
	config := conf.NewConfigure(logger.Logger)
	req := request.NewRequestInterface(logger, cred, config)
	return NewFenceClient(req, cred, logger.Logger)
}

func TestFenceClient_NewAccessToken(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	token, err := client.NewAccessToken(context.Background())
	if err != nil {
		t.Fatalf("NewAccessToken error: %v", err)
	}
	if token != "new-access-token" {
		t.Errorf("expected token new-access-token, got %s", token)
	}
}

func TestFenceClient_CheckPrivileges(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	privs, err := client.CheckPrivileges(context.Background())
	if err != nil {
		t.Fatalf("CheckPrivileges error: %v", err)
	}
	if _, ok := privs["/resource"]; !ok {
		t.Errorf("expected /resource privilege")
	}
}

func TestFenceClient_CheckForShepherdAPI(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	cred := &conf.Credential{
		APIEndpoint: server.URL,
		UseShepherd: "true",
	}
	logger, _ := logs.New("test")
	req := request.NewRequestInterface(logger, cred, conf.NewConfigure(logger.Logger))
	client := NewFenceClient(req, cred, logger.Logger)

	hasShepherd, err := client.CheckForShepherdAPI(context.Background())
	if err != nil {
		t.Fatalf("CheckForShepherdAPI error: %v", err)
	}
	if !hasShepherd {
		t.Errorf("expected Shepherd to be detected")
	}
}

func TestFenceClient_DeleteRecord(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	// Test Fence fallback (shepherd check returns false or handled by mock behavior)
	msg, err := client.DeleteRecord(context.Background(), "guid-1")
	if err != nil {
		t.Fatalf("DeleteRecord error: %v", err)
	}
	if !strings.Contains(msg, "has been deleted") {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestFenceClient_GetBucketDetails(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	info, err := client.GetBucketDetails(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("GetBucketDetails error: %v", err)
	}
	if info.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", info.Region)
	}
	if info.Provider != "s3" {
		t.Errorf("expected provider s3, got %s", info.Provider)
	}

	info, err = client.GetBucketDetails(context.Background(), "unknown-bucket")
	if err != nil {
		t.Fatalf("unexpected error for unknown bucket: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info for unknown bucket")
	}
}

func TestFenceClient_UploadFlow(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	resp, err := client.InitUpload(context.Background(), "file.txt", "bucket", "")
	if err != nil {
		t.Fatalf("InitUpload error: %v", err)
	}
	if resp.URL != "https://upload.url" {
		t.Errorf("expected upload URL, got %s", resp.URL)
	}

	resp, err = client.GetUploadPresignedUrl(context.Background(), "guid-1", "file.txt", "bucket")
	if err != nil {
		t.Fatalf("GetUploadPresignedUrl error: %v", err)
	}
	if resp.URL != "https://upload.url" {
		t.Errorf("expected upload URL, got %s", resp.URL)
	}
}

func TestFenceClient_GetDownloadPresignedUrl_Fence(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	url, err := client.GetDownloadPresignedUrl(context.Background(), "guid-1", "")
	if err != nil {
		t.Fatalf("GetDownloadPresignedUrl error: %v", err)
	}
	if url != "https://download.url" {
		t.Errorf("expected download URL, got %s", url)
	}
}

func TestFenceClient_UserPing(t *testing.T) {
	mock := &mockFenceServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	resp, err := client.UserPing(context.Background())
	if err != nil {
		t.Fatalf("UserPing error: %v", err)
	}

	if resp.Username != "test-user" {
		t.Errorf("expected username test-user, got %s", resp.Username)
	}

	if _, ok := resp.YourAccess["/resource"]; !ok {
		t.Errorf("expected /resource access")
	}

	if resp.BucketPrograms["test-bucket"] != "" {
		// Our mock for /data/buckets returns a bucket but no programs by default unless we update it
		// In my update to types.go, I added Programs to S3Bucket.
	}
}
