package kinesis

import (
	"context"
	"fmt"
	"time"

	"github.com/srcfoundry/kinesis/anylogger"
	"github.com/srcfoundry/kinesis/component"
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
	anylogger.Info(fmt.Sprintf("%s last executed on: %s\n", a, a.LastExecutedDateTime))
	anylogger.Info(fmt.Sprintf("Number of times %s executed: %v\n", a, a.PreviousExecutions))
	a.LastExecutedDateTime = time.Now().Format("January 02, 2006 15:04:05 PM")
	a.PreviousExecutions++
	return nil
}
