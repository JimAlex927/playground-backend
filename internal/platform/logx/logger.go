package logx

import (
	"errors"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg Config) (*zap.Logger, func(), error) {
	normalized := DefaultConfig()
	mergeConfig(&normalized, cfg)

	logger, err := buildLogger(normalized)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = logger.Sync()
	}

	return logger, cleanup, nil
}

func buildLogger(cfg Config) (*zap.Logger, error) {
	if cfg.Format == "" {
		cfg.Format = "text"
	}
	if cfg.TimeLayout == "" {
		cfg.TimeLayout = time.RFC3339Nano
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format(cfg.TimeLayout))
		},
	}

	if cfg.Format == "text" || cfg.Format == "console" {
		if cfg.Color {
			encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		} else {
			encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		}
	} else {
		encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	}

	var encoder zapcore.Encoder
	switch cfg.Format {
	case "text", "console":
		encoder = zapcore.NewConsoleEncoder(encCfg)
	case "json":
		encoder = zapcore.NewJSONEncoder(encCfg)
	default:
		return nil, errors.New("invalid log format, use text/console/json")
	}

	cores, err := buildCores(cfg, encoder)
	if err != nil {
		return nil, err
	}

	options := []zap.Option{
		zap.AddCaller(),
	}
	if !cfg.AddSource {
		options = nil
	}
	if cfg.Development {
		options = append(options, zap.Development(), zap.AddStacktrace(zapcore.WarnLevel))
	} else {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	logger := zap.New(zapcore.NewTee(cores...), options...)
	if cfg.ServiceName != "" {
		logger = logger.Named(cfg.ServiceName)
	}
	return logger, nil
}

func buildCores(cfg Config, encoder zapcore.Encoder) ([]zapcore.Core, error) {
	minLevel := cfg.level()
	cores := make([]zapcore.Core, 0, 2)

	if cfg.Stdout {
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.AddSync(os.Stdout),
			zap.LevelEnablerFunc(func(level zapcore.Level) bool { return level >= minLevel }),
		))
	}

	if cfg.File.Enabled {
		if cfg.File.SeparateLevel {
			specs := []struct {
				tag string
				min zapcore.Level
				max zapcore.Level
			}{
				{"DEBUG", zapcore.DebugLevel, zapcore.InfoLevel},
				{"INFO", zapcore.InfoLevel, zapcore.WarnLevel},
				{"WARN", zapcore.WarnLevel, zapcore.ErrorLevel},
				{"ERROR", zapcore.ErrorLevel, zapcore.InvalidLevel},
			}
			for _, spec := range specs {
				if minLevel > spec.min && spec.max != zapcore.InvalidLevel {
					continue
				}
				ws, err := buildWriteSyncer(cfg, spec.tag)
				if err != nil {
					return nil, err
				}

				var enabler zapcore.LevelEnabler
				if spec.max == zapcore.InvalidLevel {
					enabler = zap.LevelEnablerFunc(func(level zapcore.Level) bool {
						return level >= maxLevel(minLevel, zapcore.ErrorLevel)
					})
				} else {
					enabler = levelRange{min: maxLevel(minLevel, spec.min), max: spec.max}
				}
				cores = append(cores, zapcore.NewCore(encoder, ws, enabler))
			}
		} else {
			ws, err := buildWriteSyncer(cfg, "")
			if err != nil {
				return nil, err
			}
			cores = append(cores, zapcore.NewCore(
				encoder,
				ws,
				zap.LevelEnablerFunc(func(level zapcore.Level) bool { return level >= minLevel }),
			))
		}
	}

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel))
	}
	return cores, nil
}

func buildWriteSyncer(cfg Config, levelTag string) (zapcore.WriteSyncer, error) {
	writer, err := NewDailyWriter(
		cfg.File.Dir,
		cfg.ServiceName,
		levelTag,
		cfg.File.DailyRotate,
		cfg.File.AlsoBySize,
		cfg.File.MaxSizeMB,
		cfg.File.MaxBackups,
		cfg.File.MaxAgeDays,
		cfg.File.Compress,
	)
	if err != nil {
		return nil, err
	}
	return zapcore.AddSync(writerWithSync{writer}), nil
}

type writerWithSync struct{ *DailyWriter }

func (w writerWithSync) Write(p []byte) (int, error) { return w.DailyWriter.Write(p) }
func (w writerWithSync) Sync() error                 { return w.DailyWriter.Sync() }

type levelRange struct {
	min zapcore.Level
	max zapcore.Level
}

func (l levelRange) Enabled(level zapcore.Level) bool {
	return level >= l.min && level < l.max
}

func maxLevel(a, b zapcore.Level) zapcore.Level {
	if a > b {
		return a
	}
	return b
}

func mergeConfig(target *Config, source Config) {
	if source.ServiceName != "" {
		target.ServiceName = source.ServiceName
	}
	if source.Level != "" {
		target.Level = source.Level
	}
	if source.Format != "" {
		target.Format = source.Format
	}
	target.Stdout = source.Stdout
	target.Color = source.Color
	target.AddSource = source.AddSource
	target.Development = source.Development
	if source.TimeLayout != "" {
		target.TimeLayout = source.TimeLayout
	}

	target.File.Enabled = source.File.Enabled
	if source.File.Dir != "" {
		target.File.Dir = source.File.Dir
	}
	target.File.SeparateLevel = source.File.SeparateLevel
	target.File.DailyRotate = source.File.DailyRotate
	target.File.AlsoBySize = source.File.AlsoBySize
	if source.File.MaxSizeMB > 0 {
		target.File.MaxSizeMB = source.File.MaxSizeMB
	}
	if source.File.MaxBackups >= 0 {
		target.File.MaxBackups = source.File.MaxBackups
	}
	if source.File.MaxAgeDays >= 0 {
		target.File.MaxAgeDays = source.File.MaxAgeDays
	}
	target.File.Compress = source.File.Compress
}
