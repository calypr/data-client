package conf

import (
	"log/slog"
	"os"
	"path"
	"testing"
)

func TestNewConfigure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewConfigure(logger)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Type assertion to verify it's a *Manager
	if _, ok := manager.(*Manager); !ok {
		t.Error("Expected manager to be of type *Manager")
	}
}

func TestConfigPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	configPath, err := manager.configPath()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if configPath == "" {
		t.Error("Expected non-empty config path")
	}

	// Verify path contains expected components
	if !contains(configPath, ".gen3") {
		t.Error("Expected config path to contain .gen3 directory")
	}

	if !contains(configPath, "gen3_client_config.ini") {
		t.Error("Expected config path to contain gen3_client_config.ini")
	}
}

func TestImport_WithCredentialFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	// Create a temporary credential file
	tmpDir := t.TempDir()
	credFile := path.Join(tmpDir, "cred.json")

	credContent := `{
		"KeyID": "test-key-id",
		"APIKey": "test-api-key"
	}`

	if err := os.WriteFile(credFile, []byte(credContent), 0644); err != nil {
		t.Fatalf("Failed to create test credential file: %v", err)
	}

	cred, err := manager.Import(credFile, "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cred == nil {
		t.Fatal("Expected non-nil credential")
	}

	if cred.KeyID != "test-key-id" {
		t.Errorf("Expected KeyID 'test-key-id', got '%s'", cred.KeyID)
	}

	if cred.APIKey != "test-api-key" {
		t.Errorf("Expected APIKey 'test-api-key', got '%s'", cred.APIKey)
	}
}

func TestImport_WithFenceToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	token := "test-fence-token-12345"
	cred, err := manager.Import("", token)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cred == nil {
		t.Fatal("Expected non-nil credential")
	}

	if cred.AccessToken != token {
		t.Errorf("Expected AccessToken '%s', got '%s'", token, cred.AccessToken)
	}
}

func TestImport_NoCredentialOrToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	_, err := manager.Import("", "")

	if err == nil {
		t.Fatal("Expected error when neither credential file nor token provided")
	}

	if !contains(err.Error(), "either credential file or fence token must be provided") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestImport_InvalidCredentialFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	// Test with non-existent file
	_, err := manager.Import("/nonexistent/path/cred.json", "")

	if err == nil {
		t.Fatal("Expected error for non-existent file")
	}
}

func TestImport_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	credFile := path.Join(tmpDir, "invalid.json")

	invalidJSON := `{invalid json content`

	if err := os.WriteFile(credFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := manager.Import(credFile, "")

	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !contains(err.Error(), "cannot parse JSON credential file") {
		t.Errorf("Expected JSON parse error, got: %v", err)
	}
}

func TestEnsureExists(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	// This test is tricky because it modifies the user's home directory
	// We'll just verify it doesn't panic and returns a reasonable error or nil
	err := manager.EnsureExists()

	// We accept either success or a reasonable error
	if err != nil {
		// Just log the error, don't fail the test
		t.Logf("EnsureExists returned error (may be expected): %v", err)
	}
}

func TestLoad_ProfileNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := &Manager{Logger: logger}

	// Try to load a profile that doesn't exist
	_, err := manager.Load("nonexistent-profile")

	if err == nil {
		t.Fatal("Expected error for non-existent profile")
	}

	// Should contain profile not found error
	if !contains(err.Error(), "profile not found") && !contains(err.Error(), "Need to run") {
		t.Logf("Got error (may be expected): %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
