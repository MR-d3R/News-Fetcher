package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ColorfulLogger struct {
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	logLevel    int
}

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
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
	}

	return &ColorfulLogger{
		infoLogger:  log.New(authLogFile, fmt.Sprintf("%s[INFO]%s ", colorGreen, colorReset), log.Ldate|log.Ltime),
		warnLogger:  log.New(authLogFile, fmt.Sprintf("%s[WARN]%s ", colorYellow, colorReset), log.Ldate|log.Ltime),
		errorLogger: log.New(authLogFile, fmt.Sprintf("%s[ERROR]%s ", colorRed, colorReset), log.Ldate|log.Ltime),
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

// LogRequest логирует информацию о HTTP запросе
func (l *ColorfulLogger) LogRequest(method, path, ip string, status int, duration time.Duration) {
	var statusColor string

	if status >= 200 && status < 300 {
		statusColor = colorGreen
	} else if status >= 300 && status < 400 {
		statusColor = colorBlue
	} else if status >= 400 && status < 500 {
		statusColor = colorYellow
	} else {
		statusColor = colorRed
	}

	logMessage := fmt.Sprintf("%s %s %s%d%s %s %s",
		method,
		path,
		statusColor,
		status,
		colorReset,
		duration.String(),
		ip)

	if status >= 400 {
		l.Warn(logMessage)
	} else {
		l.Info(logMessage)
	}
}
