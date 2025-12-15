package logs

import (
	"encoding/json"
	"fmt"
	"io" // Added for standard logging methods like Fatal
	"os"
	"path/filepath"
	"sync"

	"github.com/calypr/data-client/client/common"
)

// --- teeLogger Implementation ---
type teeLogger struct {
	mu         sync.RWMutex
	writers    []io.Writer
	scoreboard *Scoreboard

	failedMu   sync.Mutex
	FailedMap  map[string]common.RetryObject // Maps filePath to FileMetadata
	failedPath string

	succeededMu   sync.Mutex
	succeededMap  map[string]string // Maps filePath to GUID
	succeededPath string
}

// NewTeeLogger combines initialization and log loading (replacing initSyncLogs)
func NewTeeLogger(logDir, profile string, sb *Scoreboard, writers ...io.Writer) *teeLogger {
	t := &teeLogger{
		mu:         sync.RWMutex{},
		writers:    writers,
		scoreboard: sb,

		FailedMap:    make(map[string]common.RetryObject),
		succeededMap: make(map[string]string),
	}

	t.failedPath = filepath.Join(logDir, profile+"_failed_log.json")
	t.succeededPath = filepath.Join(logDir, profile+"_succeeded_log.json")

	loadJSON(t.failedPath, &t.FailedMap)
	loadJSON(t.succeededPath, &t.succeededMap)

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
func (t *teeLogger) Printf(format string, v ...any) {
	t.write(fmt.Sprintf(format, v...))
}

// Println implements part of the standard Logger interface.
func (t *teeLogger) Println(v ...any) {
	t.write(fmt.Sprintln(v...))
}

// Fatalf implements part of the standard Logger interface and exits the program.
func (t *teeLogger) Fatalf(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	t.write(s)
	os.Exit(1)
}

// Fatal implements part of the standard Logger interface and exits the program.
func (t *teeLogger) Fatal(v ...any) {
	s := fmt.Sprintln(v...)
	t.write(s)
	os.Exit(1)
}

// Writer implements part of the standard Logger interface, returning a multi-writer.
func (t *teeLogger) Writer() io.Writer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return io.MultiWriter(t.writers...)
}

// Scoreboard returns the embedded ScoreboardAccess.
func (t *teeLogger) Scoreboard() *Scoreboard {
	return t.scoreboard
}

// GetSucceededLogMap returns a copy of the succeeded log map.
func (t *teeLogger) GetSucceededLogMap() map[string]string {
	t.succeededMu.Lock()
	defer t.succeededMu.Unlock()
	// Return a copy to prevent external modification
	copiedMap := make(map[string]string, len(t.succeededMap))
	for k, v := range t.succeededMap {
		copiedMap[k] = v
	}
	return copiedMap
}

// GetFailedLogMap returns a copy of the failed log map.
func (t *teeLogger) GetFailedLogMap() map[string]common.RetryObject {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()
	// Return a copy to prevent external modification
	copiedMap := make(map[string]common.RetryObject, len(t.FailedMap))
	for k, v := range t.FailedMap {
		copiedMap[k] = v
	}
	return copiedMap
}

func (t *teeLogger) DeleteFromFailedLog(path string) {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()
	delete(t.FailedMap, path)
}

// --- Internal Utility Methods ---

// write handles writing the string to all configured writers.
func (t *teeLogger) write(s string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, w := range t.writers {
		_, _ = fmt.Fprint(w, s)
	}
}

func (t *teeLogger) GetSucceededCount() int {
	return len(t.succeededMap)
}

func (t *teeLogger) writeFailedSync(e common.RetryObject) {
	t.failedMu.Lock()
	defer t.failedMu.Unlock()

	// Store the FileMetadata part in the map
	t.FailedMap[e.FilePath] = e

	data, _ := json.MarshalIndent(t.FailedMap, "", "  ")
	os.WriteFile(t.failedPath, data, 0644)
}

func (t *teeLogger) writeSucceededSync(path, guid string) {
	t.succeededMu.Lock()
	defer t.succeededMu.Unlock()
	t.succeededMap[path] = guid
	data, _ := json.MarshalIndent(t.succeededMap, "", "  ")
	os.WriteFile(t.succeededPath, data, 0644)
}

// --- Tracking Methods (Part of Logger Interface) ---

func (t *teeLogger) Failed(filePath, filename string, metadata common.FileMetadata, guid string, retryCount int, multipart bool) {
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

func (t *teeLogger) Succeeded(filePath, guid string) {
	// Use t.succeededPath instead of checking the old global succeededPath
	if t.succeededPath != "" {
		t.writeSucceededSync(filePath, guid)
	}
}
