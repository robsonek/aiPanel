package adapter

import "context"

// PostgreSQL defines operations required to manage PostgreSQL databases and users.
type PostgreSQL interface {
	CreateDatabase(ctx context.Context, dbName string) error
	DropDatabase(ctx context.Context, dbName string) error
	CreateUser(ctx context.Context, username, password, dbName string) error
	DropUser(ctx context.Context, username string) error
	IsRunning(ctx context.Context) (bool, error)
}
