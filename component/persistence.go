package component

import (
	"context"
	"database/sql"
)

// Connection represents an interface for managing database connections.
type Connection interface {
	Connect(options ...interface{}) error
	Disconnect(options ...interface{}) error
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

type Persistence struct {
	SimpleComponent
	DB Connection
}
