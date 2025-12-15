package logs

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

func New(profile string, opts ...Option) Logger {
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

	// Create the one true logger
	l := &teeLogger{
		writers: writers,
	}

	if cfg.enableScoreboard {
		sb := NewSB(5, l)
		l.scoreboard = sb
	}

	// Optional: close message log file at end of program (CLI safe)
	if messageFile != nil {
		go func(f *os.File) {
			// Wait long enough for any operation to finish
			time.Sleep(60 * time.Second)
			f.Close()
		}(messageFile)
	}

	return l
}
