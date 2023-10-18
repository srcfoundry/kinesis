package kinesis

import (
	"context"
	"log"
	"time"

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
	log.Printf("%s last executed on: %s\n", a, a.LastExecutedDateTime)
	log.Printf("Number of times %s executed: %v\n", a, a.PreviousExecutions)
	a.LastExecutedDateTime = time.Now().Format("January 02, 2006")
	a.PreviousExecutions++
	return nil
}
