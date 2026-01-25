package logger

import (
	"erp-connector/internal/config"
	"erp-connector/internal/platform/paths"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type LoggerService interface {
	Info(msg string)
	Error(msg string, err error)
	Warn(msg string)
	Success(msg string)
	Close() error
}

type service struct {
	logger *log.Logger
	file   *os.File
}

func New(cfg config.Config) (LoggerService, error) {
	logPath, err := paths.LoggerFilePath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	var out io.Writer = f
	if cfg.Debug {
		out = io.MultiWriter(os.Stderr, f)
	}

	return &service{
		logger: log.New(out, "", log.LstdFlags),
		file:   f,
	}, nil
}

func NewStderr() LoggerService {
	return &service{
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

func (s *service) Info(msg string) {
	s.write("INFO", msg)
}

func (s *service) Error(msg string, err error) {
	msg = strings.TrimSpace(msg)
	if err != nil {
		if msg == "" {
			msg = err.Error()
		} else {
			msg = msg + ": " + err.Error()
		}
	}
	s.write("ERROR", msg)
}

func (s *service) Warn(msg string) {
	s.write("WARN", msg)
}

func (s *service) Success(msg string) {
	s.write("OK", msg)
}

func (s *service) Close() error {
	if s.file == nil {
		return nil
	}
	return s.file.Close()
}

func (s *service) write(level, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	s.logger.Printf("[%s] %s", level, msg)
}
