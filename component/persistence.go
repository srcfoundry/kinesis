package component

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Connection represents an interface for managing database connections.
type Connection interface {
	Connect(tx context.Context, options ...interface{}) error
	Disconnect(tx context.Context, options ...interface{}) error
}

// RelationalDB represents the interface for a relational database.
type RelationalDB interface {
	Connection
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	// Add other relational database methods as needed
}

// NoSQLDB represents the interface for a NoSQL database.
type NoSQLDB interface {
	Connection
	Insert(ctx context.Context, collection string, document interface{}, args ...interface{}) error
	Update(ctx context.Context, collection string, filter interface{}, update interface{}, args ...interface{}) error
	Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error
	FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error
	Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error)
	// Add other NoSQL database methods as needed
}

var runOncePersistence sync.Once

type Persistence struct {
	SimpleComponent
	DB Connection
}

// Init includes checks for singleton Persistence
func (p *Persistence) Init(ctx context.Context) error {
	isAlreadyStarted := make(chan bool, 2)
	defer close(isAlreadyStarted)

	var (
		connErr    error
		connCtx    context.Context
		connCancel context.CancelFunc
	)

	runOncePersistence.Do(func() {
		// indicate if initializing for the first time
		isAlreadyStarted <- false
		connCtx, connCancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer connCancel()

		connErr = p.DB.Connect(connCtx)
	})

	isAlreadyStarted <- true

	// check the first bool value written to the channel and return error if Persistence component had already been initialized.
	if <-isAlreadyStarted {
		return fmt.Errorf("error initializing %v since already attempted initialization", p.GetName())
	}

	// if starting for first time would have to drain the channel of remaining value before returning, to avoid memory leak
	<-isAlreadyStarted

	if connErr != nil {
		return connErr
	}

	select {
	case <-connCtx.Done():
		return connCtx.Err()
	default:
		return nil
	}
}
