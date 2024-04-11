package anylogger

import (
	"testing"

	"go.uber.org/zap"
)

func TestAssignLevelLogger(t *testing.T) {
	type args struct {
		lvlLogger LevelLogger[zap.Field]
	}

	zapLogger := zap.Must(zap.NewProduction())

	tests := []struct {
		name string
		args args
	}{
		{
			name: "AddRestartableComponent",
			args: args{
				zapLogger,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AssignLevelLogger[zap.Field](tt.args.lvlLogger)
			Info(tt.args.lvlLogger, zap.Field{})("test")
			TTY()("dfd")
		})
	}
}
