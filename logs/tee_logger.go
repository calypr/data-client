package logs

import (
	"encoding/json"
	"fmt"
	"io" // Added for standard logging methods like Fatal
	"maps"
	"os"
	"sync"

	"log/slog"

	"github.com/calypr/data-client/common"
)

// --- Gen3Logger Implementation ---
type Gen3Logger struct {
	*slog.Logger
	mu         sync.RWMutex
	scoreboard *Scoreboard

	failedMu   sync.Mutex
	FailedMap  map[string]common.RetryObject // Maps filePath to FileMetadata
	failedPath string

	succeededMu   sync.Mutex
	succeededMap  map[string]string // Maps filePath to GUID
	succeededPath string
}

// NewGen3Logger combines initialization and log loading
func NewGen3Logger(logger *slog.Logger, logDir, profile string) *Gen3Logger {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	t := &Gen3Logger{
		Logger:     logger,
		mu:         sync.RWMutex{},
		scoreboard: nil,

		FailedMap:    make(map[string]common.RetryObject),
		succeededMap: make(map[string]string),
	}

	return t
}

// Internal helper function (replaces the global loadJSON)
func loadJSON(path string, v any) {
	data, _ := os.ReadFile(path)
	if len(data) > 0 {
		// Error handling for Unmarshal is often omitted in utility code
		// but is good practice. We keep the original style for now.
		json.Unmarshal(data, v)
	}
}

// --- Public Logger Methods ---

// Printf implements part of the standard Logger interface.
// Printf implements part of the standard Logger interface.
func (t *Gen3Logger) Printf(format string, v ...any) {
	t.Info(fmt.Sprintf(format, v...))
}

// Println implements part of the standard Logger interface.
func (t *Gen3Logger) Println(v ...any) {
	t.Info(fmt.Sprint(v...))
}

// Fatalf implements part of the standard Logger interface and exits the program.
func (t *Gen3Logger) Fatalf(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	t.Error(s)
	os.Exit(1)
}

// Fatal implements part of the standard Logger interface and exits the program.
func (t *Gen3Logger) Fatal(v ...any) {
	s := fmt.Sprint(v...)
	t.Error(s)
	os.Exit(1)
}

// Writer implements part of the standard Logger interface, returning a multi-writer.
// Writer implements part of the standard Logger interface.
// For slog, accessing the underlying writer is not standard.
// We return a no-op writer or stdout appropriately if needed for legacy compatibility.
// But mostly this was used to chain loggers.
func (t *Gen3Logger) Writer() io.Writer {
	// Attempt to return a writer that logs to Info?
	// Or just return os.Stderr for now.
	return os.Stderr
}

// Scoreboard returns the embedded ScoreboardAccess.
func (t *Gen3Logger) Scoreboard() *Scoreboard {
	return t.scoreboard
}

// GetSucceededLogMap returns a copy of the succeeded log map.
func (t *Gen3Logger) GetSucceededLogMap() map[string]string {
	t.succeededMu.Lock()
	defer t.succeededMu.Unlock()
	// Return a copy to prevent external modification
	copiedMap := make(map[string]string, len(t.succeededMap))
	maps.Copy(copiedMap, t.succeededMap)

	return copiedMap
}

// GetFailedLogMap returns a copy of the failed log map.
func (t *Gen3Logger) GetFailedLogMap() map[string]common.RetryObject {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()
	// Return a copy to prevent external modification
	copiedMap := make(map[string]common.RetryObject, len(t.FailedMap))
	maps.Copy(copiedMap, t.FailedMap)
	return copiedMap
}

func (t *Gen3Logger) DeleteFromFailedLog(path string) {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()
	delete(t.FailedMap, path)
}

// --- Internal Utility Methods ---

// write handles writing the string to all configured writers.
// write is removed as we use slog directly.

func (t *Gen3Logger) GetSucceededCount() int {
	return len(t.succeededMap)
}

func (t *Gen3Logger) writeFailedSync(e common.RetryObject) {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()

	// Store the FileMetadata part in the map
	t.FailedMap[e.FilePath] = e

	data, _ := json.MarshalIndent(t.FailedMap, "", "  ")
	os.WriteFile(t.failedPath, data, 0644)
}

func (t *Gen3Logger) writeSucceededSync(path, guid string) {
	t.succeededMu.Lock()
	defer t.succeededMu.Unlock()
	t.succeededMap[path] = guid
	data, _ := json.MarshalIndent(t.succeededMap, "", "  ")
	os.WriteFile(t.succeededPath, data, 0644)
}

// --- Tracking Methods (Part of Logger Interface) ---

func (t *Gen3Logger) Failed(filePath, filename string, metadata common.FileMetadata, guid string, retryCount int, multipart bool) {
	if t.failedPath != "" {
		t.writeFailedSync(common.RetryObject{
			FilePath:     filePath,
			Filename:     filename,
			FileMetadata: metadata,
			GUID:         guid,
			RetryCount:   retryCount,
			Multipart:    multipart,
		})
	}
}

func (t *Gen3Logger) Succeeded(filePath, guid string) {
	// Use t.succeededPath instead of checking the old global succeededPath
	if t.succeededPath != "" {
		t.writeSucceededSync(filePath, guid)
	}
}
