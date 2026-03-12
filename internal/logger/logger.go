package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	instance *Logger
	once     sync.Once
)

type Logger struct {
	file *os.File
	mu   sync.Mutex
	path string
}

func Init() error {
	var initErr error
	once.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			initErr = fmt.Errorf("get home dir: %w", err)
			return
		}

		logPath := filepath.Join(home, ".bdtui", "log.file")
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			initErr = fmt.Errorf("create log dir: %w", err)
			return
		}

		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			initErr = fmt.Errorf("open log file: %w", err)
			return
		}

		instance = &Logger{
			file: file,
			path: logPath,
		}
	})

	return initErr
}

func Error(format string, args ...interface{}) {
	if instance == nil {
		return
	}
	instance.log("ERROR", format, args...)
}

func Info(format string, args ...interface{}) {
	if instance == nil {
		return
	}
	instance.log("INFO", format, args...)
}

func (l *Logger) log(level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, msg)
	l.file.WriteString(line)
}

func Close() {
	if instance != nil && instance.file != nil {
		instance.file.Close()
	}
}

func Path() string {
	if instance == nil {
		return ""
	}
	return instance.path
}
