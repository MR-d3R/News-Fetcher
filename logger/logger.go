package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type ColorfulLogger struct {
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	panicLogger *log.Logger
	debugLogger *log.Logger
	logLevel    int
}

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	PANIC
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
)

// Cоздает новый экземпляр логгера
func NewColorfulLogger(logPrefix string, logLevelStr string) (*ColorfulLogger, error) {
	logDir := "logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.Mkdir(logDir, 0755)
	}

	logFileName := fmt.Sprintf("%s.log", logPrefix)
	authLogFile, err := os.OpenFile(
		filepath.Join("logs", logFileName),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл лога %s: %v", logFileName, err)
	}

	logLevel := INFO // По умолчанию

	switch strings.ToLower(logLevelStr) {
	case "debug":
		logLevel = DEBUG
	case "info":
		logLevel = INFO
	case "warn":
		logLevel = WARN
	case "error":
		logLevel = ERROR
	case "panic":
		logLevel = PANIC
	}

	return &ColorfulLogger{
		infoLogger:  log.New(authLogFile, fmt.Sprintf("%s[INFO]%s ", colorGreen, colorReset), log.Ldate|log.Ltime),
		warnLogger:  log.New(authLogFile, fmt.Sprintf("%s[WARN]%s ", colorYellow, colorReset), log.Ldate|log.Ltime),
		errorLogger: log.New(authLogFile, fmt.Sprintf("%s[ERROR]%s ", colorRed, colorReset), log.Ldate|log.Ltime),
		panicLogger: log.New(authLogFile, fmt.Sprintf("%s[PANIC]%s ", colorBlue, colorReset), log.Ldate|log.Ltime),
		debugLogger: log.New(authLogFile, fmt.Sprintf("%s[DEBUG]%s ", colorBlue, colorReset), log.Ldate|log.Ltime),
		logLevel:    logLevel,
	}, nil
}

// Debug логирует отладочные сообщения
func (l *ColorfulLogger) Debug(format string, v ...interface{}) {
	if l.logLevel <= DEBUG {
		l.debugLogger.Printf(format, v...)
	}
}

// Info логирует информационные сообщения
func (l *ColorfulLogger) Info(format string, v ...interface{}) {
	if l.logLevel <= INFO {
		l.infoLogger.Printf(format, v...)
	}
}

// Warn логирует предупреждения
func (l *ColorfulLogger) Warn(format string, v ...interface{}) {
	if l.logLevel <= WARN {
		l.warnLogger.Printf(format, v...)
	}
}

// Error логирует ошибки
func (l *ColorfulLogger) Error(format string, v ...interface{}) {
	if l.logLevel <= ERROR {
		l.errorLogger.Printf(format, v...)
	}
}

// Panic завершает работу приложения
func (l *ColorfulLogger) Panic(format string, v ...interface{}) {
	if l.logLevel <= PANIC {
		l.errorLogger.Printf(format, v...)
		panic(v)
	}
}
