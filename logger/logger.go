package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Level represents log level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger represents a logger instance
type Logger struct {
	file   *os.File
	level  Level
	debug  bool
}

// global logger instance
var global *Logger

// Init initializes the logger
func Init(debug bool) error {
	if !debug {
		return nil
	}

	// Create logs directory
	logDir := filepath.Join(".", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("gopilot_%s.log", timestamp))

	file, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	global = &Logger{
		file:  file,
		level: LevelDebug,
		debug: true,
	}

	// Write header
	global.write("INFO", "Logger initialized", "file", logPath)

	return nil
}

// Close closes the logger
func Close() {
	if global != nil && global.file != nil {
		global.write("INFO", "Logger closed", "")
		global.file.Close()
	}
}

// Debug logs a debug message
func Debug(msg string, keysAndValues ...interface{}) {
	if global == nil {
		return
	}
	global.write("DEBUG", msg, keysAndValues...)
}

// Info logs an info message
func Info(msg string, keysAndValues ...interface{}) {
	if global == nil {
		return
	}
	global.write("INFO", msg, keysAndValues...)
}

// Warn logs a warning message
func Warn(msg string, keysAndValues ...interface{}) {
	if global == nil {
		return
	}
	global.write("WARN", msg, keysAndValues...)
}

// Error logs an error message
func Error(msg string, keysAndValues ...interface{}) {
	if global == nil {
		return
	}
	global.write("ERROR", msg, keysAndValues...)
}

// IsDebug returns true if debug mode is enabled
func IsDebug() bool {
	return global != nil && global.debug
}

func (l *Logger) write(level, msg string, keysAndValues ...interface{}) {
	if l.file == nil {
		return
	}

	// Get caller info
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	} else {
		file = filepath.Base(file)
	}

	// Format timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Build log line
	logLine := fmt.Sprintf("[%s] [%s:%d] %s", timestamp, file, line, msg)

	// Add key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			logLine += fmt.Sprintf(" %s=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}

	logLine += "\n"

	// Write to file
	l.file.WriteString(logLine)
	l.file.Sync()
}
