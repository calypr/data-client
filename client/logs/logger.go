package logs

import (
	"io"
)

type Logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
	Fatalf(format string, v ...any)
	Fatal(v ...any)
	Writer() io.Writer
}

type Option func(*config)

type config struct {
	console          bool
	messageFile      bool
	failedLog        bool
	succeededLog     bool
	enableScoreboard bool
	baseLogger       Logger
}

func WithConsole() Option               { return func(c *config) { c.console = true } }
func WithMessageFile() Option           { return func(c *config) { c.messageFile = true } }
func WithFailedLog() Option             { return func(c *config) { c.failedLog = true } }
func WithSucceededLog() Option          { return func(c *config) { c.succeededLog = true } }
func WithScoreboard() Option            { return func(c *config) { c.enableScoreboard = true } }
func WithBaseLogger(base Logger) Option { return func(c *config) { c.baseLogger = base } }

func defaults() *config {
	return &config{
		console:      true,
		messageFile:  true,
		failedLog:    true,
		succeededLog: true,
		baseLogger:   nil,
	}
}
