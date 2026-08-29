// Structured logging, bounded: a single concrete text-format Logger, no
// pluggable Handler interface (real slog's Handler/JSONHandler/
// TextHandler split), no Group/LogAttrs/context-aware variants. Output
// is "LEVEL msg key=value key=value ...", not real slog's actual
// key=value quoting/escaping rules -- close enough for real use, not a
// byte-for-byte match. `args ...any` alternates key, value, key, value,
// same convention as real slog's Info/Warn/etc.
package slog

import (
	"fmt"
	"io"
	"os"
)

type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// LevelString formats a Level, e.g. "INFO" -- not a Level.String()
// method: wasigoc only supports methods on a real struct receiver, and
// Level (`type Level int`) is a `using` alias, not one -- same reason
// time.Duration is spelled FormatDuration(d) here instead of a method.
func LevelString(l Level) string {
	if l == LevelDebug {
		return "DEBUG"
	}
	if l == LevelInfo {
		return "INFO"
	}
	if l == LevelWarn {
		return "WARN"
	}
	if l == LevelError {
		return "ERROR"
	}
	return "UNKNOWN"
}

type Logger struct {
	w        io.Writer
	minLevel Level
}

func New(w io.Writer) *Logger {
	return &Logger{w: w, minLevel: LevelInfo}
}

func (l *Logger) SetLevel(level Level) {
	l.minLevel = level
}

func (l *Logger) log(level Level, msg string, args []any) {
	if level < l.minLevel {
		return
	}
	line := LevelString(level) + " " + msg
	i := 0
	for i+1 < len(args) {
		key := fmt.Sprintf("%v", args[i])
		val := fmt.Sprintf("%v", args[i+1])
		line = line + " " + key + "=" + val
		i = i + 2
	}
	fmt.Fprintln(l.w, line)
}

func (l *Logger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args) }
func (l *Logger) Info(msg string, args ...any)  { l.log(LevelInfo, msg, args) }
func (l *Logger) Warn(msg string, args ...any)  { l.log(LevelWarn, msg, args) }
func (l *Logger) Error(msg string, args ...any) { l.log(LevelError, msg, args) }

var defaultLogger = New(os.Stdout)

func Default() *Logger {
	return defaultLogger
}

func SetDefault(l *Logger) {
	defaultLogger = l
}

func Debug(msg string, args ...any) { defaultLogger.Debug(msg, args...) }
func Info(msg string, args ...any)  { defaultLogger.Info(msg, args...) }
func Warn(msg string, args ...any)  { defaultLogger.Warn(msg, args...) }
func Error(msg string, args ...any) { defaultLogger.Error(msg, args...) }
