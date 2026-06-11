package request

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/logs"
)

func TestNewRequestInterface(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: "https://example.com",
	}

	// Create a mock config manager
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)

	if reqInterface == nil {
		t.Fatal("Expected non-nil request interface")
	}

	req, ok := reqInterface.(*Request)
	if !ok {
		t.Fatal("Expected request interface to be of type *Request")
	}

	if req.RetryClient == nil {
		t.Error("Expected non-nil retry client")
	}

	if req.Logs == nil {
		t.Error("Expected non-nil logger")
	}
}

func TestRequestBuilder_New(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: "https://example.com",
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	builder := req.New("GET", "https://example.com/api/test")

	if builder == nil {
		t.Fatal("Expected non-nil request builder")
	}

	if builder.Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", builder.Method)
	}

	if builder.Url != "https://example.com/api/test" {
		t.Errorf("Expected URL 'https://example.com/api/test', got '%s'", builder.Url)
	}
}

func TestRequestBuilder_WithHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: "https://example.com",
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	builder := req.New("GET", "https://example.com/api/test")
	builder = builder.WithHeader("Content-Type", "application/json")
	builder = builder.WithHeader("X-Custom-Header", "test-value")

	if len(builder.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(builder.Headers))
	}

	if builder.Headers["Content-Type"] != "application/json" {
		t.Error("Expected Content-Type header to be set")
	}

	if builder.Headers["X-Custom-Header"] != "test-value" {
		t.Error("Expected X-Custom-Header to be set")
	}
}

func TestRequestBuilder_WithToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: "https://example.com",
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	token := "test-bearer-token-12345"
	builder := req.New("GET", "https://example.com/api/test")
	builder = builder.WithToken(token)

	if builder.Token != token {
		t.Errorf("Expected token '%s', got '%s'", token, builder.Token)
	}
}

func TestRequestBuilder_WithBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: "https://example.com",
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	body := strings.NewReader("test body content")
	builder := req.New("POST", "https://example.com/api/test")
	builder = builder.WithBody(body)

	if builder.Body == nil {
		t.Error("Expected non-nil body")
	}
}

func TestRequest_Do_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: server.URL,
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	builder := req.New("GET", server.URL+"/api/test")
	builder = builder.WithToken("test-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := req.Do(ctx, builder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "success") {
		t.Error("Expected response body to contain 'success'")
	}
}

func TestRequest_Do_WithCustomHeaders(t *testing.T) {
	// Create a test server that checks for custom headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customHeader := r.Header.Get("X-Custom-Header")
		if customHeader != "test-value" {
			t.Errorf("Expected X-Custom-Header 'test-value', got '%s'", customHeader)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{
		KeyID:       "test-key",
		APIKey:      "test-secret",
		APIEndpoint: server.URL,
	}
	mockConf := &mockConfigManager{}

	reqInterface := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, mockConf)
	req := reqInterface.(*Request)

	builder := req.New("GET", server.URL+"/api/test")
	builder = builder.WithHeader("X-Custom-Header", "test-value")

	ctx := context.Background()
	resp, err := req.Do(ctx, builder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	resp.Body.Close()
}

func TestRequest_JoinURL(t *testing.T) {
	got := JoinURL("https://example.org/base", "config", "explorer", "default")
	if got != "https://example.org/base/config/explorer/default" {
		t.Fatalf("unexpected joined URL: %s", got)
	}
}

func TestRequest_DecodeJSON(t *testing.T) {
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
	}

	var decoded struct {
		Status string `json:"status"`
	}
	if err := DecodeJSON(resp, &decoded); err != nil {
		t.Fatalf("DecodeJSON failed: %v", err)
	}
	if decoded.Status != "ok" {
		t.Fatalf("unexpected decoded payload: %+v", decoded)
	}
}

func TestRequest_StatusError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":"bad request"}`)),
	}

	err := StatusError(resp, "request failed")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `request failed: status 400, body: {"error":"bad request"}` {
		t.Fatalf("unexpected error: %s", got)
	}
}

type testErrorEnvelope struct {
	Type    string         `json:"type,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (t testErrorEnvelope) ErrorMessage() string {
	return t.Message
}

func (t testErrorEnvelope) ErrorType() string {
	return t.Type
}

func (t testErrorEnvelope) ErrorDetails() map[string]any {
	return t.Details
}

func TestRequest_StatusErrorJSON(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"type":"config_not_found","message":"missing","details":{"config_id":"default"}}`)),
	}

	err := StatusErrorJSON(resp, "request failed", &testErrorEnvelope{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `request failed: status 404: missing` {
		t.Fatalf("unexpected error: %s", got)
	}

	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T", err)
	}
	if statusErr.Type != "config_not_found" {
		t.Fatalf("unexpected error type: %q", statusErr.Type)
	}
	if statusErr.Details["config_id"] != "default" {
		t.Fatalf("unexpected error details: %+v", statusErr.Details)
	}
}

func TestRequest_NewJSONAndDoJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected application/json content type, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cred := &conf.Credential{KeyID: "test-key", APIKey: "test-secret", APIEndpoint: server.URL}
	req := NewRequestInterface(logs.NewGen3Logger(logger, "", ""), cred, &mockConfigManager{})

	rb, err := NewJSON(req, http.MethodPost, server.URL+"/api/test", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("NewJSON failed: %v", err)
	}

	var decoded struct {
		Status string `json:"status"`
	}
	if err := DoJSON(context.Background(), req, rb, &decoded, WithExpectedStatus(http.StatusOK), WithAction("post test")); err != nil {
		t.Fatalf("DoJSON failed: %v", err)
	}
	if decoded.Status != "ok" {
		t.Fatalf("unexpected decoded payload: %+v", decoded)
	}
}

// Mock config manager for testing
type mockConfigManager struct{}

func (m *mockConfigManager) Import(filePath, fenceToken string) (*conf.Credential, error) {
	return &conf.Credential{}, nil
}

func (m *mockConfigManager) Load(profile string) (*conf.Credential, error) {
	return &conf.Credential{}, nil
}

func (m *mockConfigManager) Save(cred *conf.Credential) error {
	return nil
}

func (m *mockConfigManager) EnsureExists() error {
	return nil
}

func (m *mockConfigManager) IsCredentialValid(cred *conf.Credential) (bool, error) {
	return true, nil
}

func (m *mockConfigManager) IsTokenValid(token string) (bool, error) {
	return true, nil
}
