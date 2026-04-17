package logx

import (
	"time"

	"go.uber.org/zap/zapcore"
)

type FileConfig struct {
	Enabled       bool
	Dir           string
	SeparateLevel bool
	DailyRotate   bool
	AlsoBySize    bool
	MaxSizeMB     int
	MaxBackups    int
	MaxAgeDays    int
	Compress      bool
}

type Config struct {
	ServiceName string
	Level       string
	Format      string
	Stdout      bool
	Color       bool
	AddSource   bool
	Development bool
	TimeLayout  string
	File        FileConfig
}

func DefaultConfig() Config {
	return Config{
		ServiceName: "app",
		Level:       "info",
		Format:      "text",
		Stdout:      true,
		Color:       true,
		AddSource:   true,
		Development: false,
		TimeLayout:  time.RFC3339Nano,
		File: FileConfig{
			Enabled:       false,
			Dir:           "storage/logs",
			SeparateLevel: false,
			DailyRotate:   true,
			AlsoBySize:    false,
			MaxSizeMB:     128,
			MaxBackups:    14,
			MaxAgeDays:    14,
			Compress:      false,
		},
	}
}

func (c Config) level() zapcore.Level {
	switch c.Level {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
