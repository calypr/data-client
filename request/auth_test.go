package request

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/logs"
)

type trackingConfigManager struct {
	mockConfigManager
	saveCalls int
	lastSaved *conf.Credential
}

func (m *trackingConfigManager) Save(cred *conf.Credential) error {
	m.saveCalls++
	if cred != nil {
		copyCred := *cred
		m.lastSaved = &copyCred
	}
	return nil
}

func TestRequestRefreshesExpiredBearerTokenOnUnauthorized(t *testing.T) {
	const (
		oldToken = "expired-token"
		newToken = "fresh-token"
		apiKey   = "refresh-api-key"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/credentials/api/access_token":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read refresh body: %v", err)
			}
			if !strings.Contains(string(body), apiKey) {
				t.Fatalf("refresh request missing api key: %s", string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"` + newToken + `"}`))
		case "/protected":
			switch r.Header.Get("Authorization") {
			case "Bearer " + oldToken:
				http.Error(w, "expired", http.StatusUnauthorized)
			case "Bearer " + newToken:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		Profile:     "test",
		APIKey:      apiKey,
		AccessToken: oldToken,
		APIEndpoint: server.URL,
	}
	cfg := &trackingConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, cfg)
	req := reqInterface.(*Request)
	builder := req.New(http.MethodGet, server.URL+"/protected")

	resp, err := req.Do(context.Background(), builder)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if cred.AccessToken != newToken {
		t.Fatalf("credential token = %q, want %q", cred.AccessToken, newToken)
	}
	if cfg.saveCalls != 1 {
		t.Fatalf("save calls = %d, want 1", cfg.saveCalls)
	}
	if cfg.lastSaved == nil || cfg.lastSaved.AccessToken != newToken {
		t.Fatalf("saved credential token = %#v, want %q", cfg.lastSaved, newToken)
	}
}
