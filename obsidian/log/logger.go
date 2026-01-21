// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package log

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/log"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelCrit
)

// Logger provides a simple logging interface wrapping go-ethereum's logger
type Logger struct {
	mu    sync.RWMutex
	level LogLevel
}

// New creates a new logger with default info level
func New() *Logger {
	return &Logger{
		level: LevelInfo,
	}
}

// SetLevel sets the logging level from a string
func (l *Logger) SetLevel(levelStr string) error {
	level, err := StringToLevel(levelStr)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
	return nil
}

// GetLevel returns the current logging level as a string
func (l *Logger) GetLevel() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	switch l.level {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelCrit:
		return "crit"
	default:
		return "info"
	}
}

// Trace logs a trace level message
func (l *Logger) Trace(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelTrace {
		log.Trace(msg, ctx...)
	}
}

// Debug logs a debug level message
func (l *Logger) Debug(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelDebug {
		log.Debug(msg, ctx...)
	}
}

// Info logs an info level message
func (l *Logger) Info(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelInfo {
		log.Info(msg, ctx...)
	}
}

// Warn logs a warning level message
func (l *Logger) Warn(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelWarn {
		log.Warn(msg, ctx...)
	}
}

// Error logs an error level message
func (l *Logger) Error(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelError {
		log.Error(msg, ctx...)
	}
}

// Crit logs a critical level message
func (l *Logger) Crit(msg string, ctx ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.level <= LevelCrit {
		log.Crit(msg, ctx...)
	}
}

// StringToLevel converts a level string to LogLevel
func StringToLevel(levelStr string) (LogLevel, error) {
	switch strings.ToLower(levelStr) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "crit", "critical":
		return LevelCrit, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level: %s", levelStr)
	}
}

// ContextLogger provides structured logging with context
type ContextLogger struct {
	logger *Logger
	ctx    []interface{}
}

// NewContextLogger creates a new context logger
func NewContextLogger(logger *Logger, ctx ...interface{}) *ContextLogger {
	return &ContextLogger{
		logger: logger,
		ctx:    ctx,
	}
}

// WithFields adds fields to the context
func (cl *ContextLogger) WithFields(ctx ...interface{}) *ContextLogger {
	return &ContextLogger{
		logger: cl.logger,
		ctx:    append(cl.ctx, ctx...),
	}
}

// Info logs with context
func (cl *ContextLogger) Info(msg string, ctx ...interface{}) {
	all := append(cl.ctx, ctx...)
	cl.logger.Info(msg, all...)
}

// Error logs error with context
func (cl *ContextLogger) Error(msg string, ctx ...interface{}) {
	all := append(cl.ctx, ctx...)
	cl.logger.Error(msg, all...)
}

// Warn logs warning with context
func (cl *ContextLogger) Warn(msg string, ctx ...interface{}) {
	all := append(cl.ctx, ctx...)
	cl.logger.Warn(msg, all...)
}

// Debug logs debug with context
func (cl *ContextLogger) Debug(msg string, ctx ...interface{}) {
	all := append(cl.ctx, ctx...)
	cl.logger.Debug(msg, all...)
}
