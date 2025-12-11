package logs

import (
	"io"
	"log"
	"os"
)

type Logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
	Fatalf(format string, v ...any)
	Fatal(v ...any)
	Writer() io.Writer
}

type defaultLogger struct{}

func (defaultLogger) Printf(format string, v ...any) { log.Printf(format, v...) }
func (defaultLogger) Println(v ...any)               { log.Println(v...) }
func (defaultLogger) Fatalf(format string, v ...any) { log.Fatalf(format, v...) }
func (defaultLogger) Fatal(v ...any)                 { log.Fatal(v...) }
func (defaultLogger) Writer() io.Writer              { return os.Stdout }

func Default() Logger {
	return defaultLogger{}
}
