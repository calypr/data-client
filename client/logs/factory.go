package logs

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

func New(profile string, opts ...Option) (*TeeLogger, func()) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	usr, _ := user.Current()
	logDir := filepath.Join(usr.HomeDir, ".gen3", "logs")
	os.MkdirAll(logDir, 0755)

	var writers []io.Writer
	var messageFile *os.File

	if cfg.baseLogger != nil {
		writers = append(writers, cfg.baseLogger.Writer())
	}

	if cfg.console {
		writers = append(writers, os.Stderr)
	}

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
		t.failedPath = filepath.Join(logDir, profile+"_failed.json")
		loadJSON(t.failedPath, &t.FailedMap)
	}

	if cfg.succeededLog {
		t.succeededPath = filepath.Join(logDir, profile+"_succeeded.json")
		loadJSON(t.succeededPath, &t.succeededMap)
	}

	cleanup := func() {
		if messageFile != nil {
			fmt.Fprintf(messageFile, "[%s] Message log stopped\n", time.Now().Format(time.RFC3339))
			messageFile.Close()
		}
	}

	return t, cleanup
}
