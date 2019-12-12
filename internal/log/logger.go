package log

import (
	"fmt"
	"log"
	"time"
)

const (
	DEBUG = iota
	INFO  = iota
	WARN  = iota
	ERROR = iota
)

var LogLevel = INFO

type Logger struct {
	name   string
	logger *log.Logger
}

func New(name string) *Logger {
	return &Logger{
		name:   " [" + name + "] ",
		logger: log.New(log.Writer(), "", 0),
	}
}

func (l *Logger) Debug(v ...interface{}) {
	if LogLevel <= DEBUG {
		l.logger.Print(date(), l.name, fmt.Sprint(v...))
	}
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if LogLevel <= DEBUG {
		l.logger.Print(date(), l.name, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Info(v ...interface{}) {
	if LogLevel <= INFO {
		l.logger.Print(date(), l.name, fmt.Sprint(v...))
	}
}

func (l *Logger) Infof(format string, v ...interface{}) {
	if LogLevel <= INFO {
		l.logger.Print(date(), l.name, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Warn(v ...interface{}) {
	if LogLevel <= WARN {
		l.logger.Print(date(), l.name, fmt.Sprint(v...))
	}
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	if LogLevel <= WARN {
		l.logger.Print(date(), l.name, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Error(v ...interface{}) {
	if LogLevel <= ERROR {
		l.logger.Print(date(), l.name, fmt.Sprint(v...))
	}
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	if LogLevel <= ERROR {
		l.logger.Print(date(), l.name, fmt.Sprintf(format, v...))
	}
}

func date() string {
	return time.Now().UTC().Format("Mon Jan _2 15:04:05 2006")
}
