package adapter

import "context"

// MariaDB defines operations required to manage MariaDB databases and users.
type MariaDB interface {
	CreateDatabase(ctx context.Context, dbName string) error
	DropDatabase(ctx context.Context, dbName string) error
	CreateUser(ctx context.Context, username, password, dbName string) error
	DropUser(ctx context.Context, username string) error
	IsRunning(ctx context.Context) (bool, error)
}
