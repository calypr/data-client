package logs

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

func New(profile string, opts ...Option) (Logger, func()) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	// Setup log directory
	usr, _ := user.Current()
	logDir := filepath.Join(usr.HomeDir, ".gen3", "logs")
	os.MkdirAll(logDir, 0755)

	// Console + message log file
	var writers []io.Writer
	if cfg.console {
		writers = append(writers, os.Stderr)
	}

	var messageFile = (*os.File)(nil)
	if cfg.messageFile {
		filename := fmt.Sprintf("%s_message_%s_%d.log",
			profile,
			time.Now().Format("20060102150405MST"),
			os.Getpid(),
		)
		f, err := os.OpenFile(filepath.Join(logDir, filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			messageFile = f
			writers = append(writers, f)
			fmt.Fprintf(f, "[%s] Message log started\n", time.Now().Format(time.RFC3339))
		}
	}

	t := NewTeeLogger(logDir, profile, writers...)
	if cfg.enableScoreboard {
		t.scoreboard = NewSB(5, t)
	}

	if cfg.failedLog {
		// Only set the path if failedLog is enabled
		t.failedPath = filepath.Join(logDir, profile+"_failed_log.json")
		loadJSON(t.failedPath, &t.FailedMap) // Loads only if enabled
	}
	if cfg.succeededLog {
		// Only set the path if succeededLog is enabled
		t.succeededPath = filepath.Join(logDir, profile+"_succeeded_log.json")
		loadJSON(t.succeededPath, &t.succeededMap) // Loads only if enabled
	}

	cleanup := func() {
		if messageFile != nil {
			fmt.Fprintf(messageFile, "[%s] Message log stopped\n", time.Now().Format(time.RFC3339))
			messageFile.Close()
		}
	}

	return t, cleanup

}
