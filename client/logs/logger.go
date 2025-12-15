package logs

import (
	"io"

	"github.com/calypr/data-client/client/common"
)

type Logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
	Fatalf(format string, v ...any)
	Fatal(v ...any)
	Writer() io.Writer

	Failed(filePath, filename string, metadata common.FileMetadata, guid string, retryCount int, multipart bool)
	Succeeded(filePath, guid string)
	Scoreboard() *Scoreboard
	GetSucceededLogMap() map[string]string
	GetFailedLogMap() map[string]common.RetryObject
	DeleteFromFailedLog(filePath string)
}

type Option func(*config)

type config struct {
	console          bool
	messageFile      bool
	failedLog        bool
	succeededLog     bool
	enableScoreboard bool
}

func WithConsole() Option      { return func(c *config) { c.console = true } }
func WithMessageFile() Option  { return func(c *config) { c.messageFile = true } }
func WithFailedLog() Option    { return func(c *config) { c.failedLog = true } }
func WithSucceededLog() Option { return func(c *config) { c.succeededLog = true } }
func WithScoreboard() Option   { return func(c *config) { c.enableScoreboard = true } }

func defaults() *config {
	return &config{
		console:      true,
		messageFile:  true,
		failedLog:    true,
		succeededLog: true,
	}
}
