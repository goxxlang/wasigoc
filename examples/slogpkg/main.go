package main

import (
	"bytes"
	"fmt"
	"log/slog"
)

func main() {
	var buf bytes.Buffer
	logger := slog.New(&buf)

	logger.Info("server started", "port", 8080)
	logger.Warn("cache miss", "key", "user:42")
	logger.Error("failed", "err", "connection refused")

	fmt.Println(buf.String() ==
		"INFO server started port=8080\nWARN cache miss key=user:42\nERROR failed err=connection refused\n")

	// Below the min level (default Info): Debug is suppressed.
	var buf2 bytes.Buffer
	logger2 := slog.New(&buf2)
	logger2.Debug("should not appear", "x", 1)
	fmt.Println(buf2.String() == "")

	// Raising the level to Debug makes it appear.
	logger2.SetLevel(slog.LevelDebug)
	logger2.Debug("now appears", "x", 1)
	fmt.Println(buf2.String() == "DEBUG now appears x=1\n")

	// LevelString.
	fmt.Println(slog.LevelString(slog.LevelDebug) == "DEBUG")
	fmt.Println(slog.LevelString(slog.LevelInfo) == "INFO")
	fmt.Println(slog.LevelString(slog.LevelWarn) == "WARN")
	fmt.Println(slog.LevelString(slog.LevelError) == "ERROR")

	// No key-value pairs at all still works.
	var buf3 bytes.Buffer
	logger3 := slog.New(&buf3)
	logger3.Info("plain message")
	fmt.Println(buf3.String() == "INFO plain message\n")

	// Package-level default-logger functions, redirected to a captured
	// buffer via SetDefault so the output is actually checked instead
	// of just "didn't crash".
	var buf4 bytes.Buffer
	slog.SetDefault(slog.New(&buf4))
	slog.Info("via default logger", "a", 1)
	fmt.Println(buf4.String() == "INFO via default logger a=1\n")
}
