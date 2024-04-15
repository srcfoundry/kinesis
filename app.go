package kinesis

import (
	"context"
	"time"

	"github.com/srcfoundry/kinesis/component"
	"go.uber.org/zap"
)

type App struct {
	component.Container
	PreviousExecutions   int    `persistable:"native"`
	LastExecutedDateTime string `persistable:"native"`
}

func (a *App) PostInit(context.Context) error {
	if len(a.LastExecutedDateTime) <= 0 {
		a.LastExecutedDateTime = "--"
	}
	a.GetLogger().Info("last executed", zap.String("component", a.GetName()), zap.String("date/time", a.LastExecutedDateTime))
	a.GetLogger().Info("number of times executed", zap.String("component", a.GetName()), zap.Int("executed", a.PreviousExecutions))
	a.LastExecutedDateTime = time.Now().Format("January 02, 2006 15:04:05 PM")
	a.PreviousExecutions++
	return nil
}
