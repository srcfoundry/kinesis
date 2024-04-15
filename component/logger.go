package component

import (
	"context"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger *zap.Logger
)

type loggerKey struct{}

func ContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func LoggerFromContext(ctx context.Context) *zap.Logger {
	if ctxLogger, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok {
		return ctxLogger
	} else if logger != nil {
		return logger
	}
	return zap.NewNop()
}

func createLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.RFC3339TimeEncoder

	// check log level at initialization
	var zapAtomicLvl zapcore.Level
	logLvl := strings.ToLower(os.Getenv("KINESIS_LOG_LEVEL"))

	switch logLvl {
	case "debug":
		zapAtomicLvl = zap.DebugLevel
	default:
		zapAtomicLvl = zap.InfoLevel
	}

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(zapAtomicLvl),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          "json",
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stderr",
		},
		ErrorOutputPaths: []string{
			"stderr",
		},
	}

	return zap.Must(config.Build())
}
