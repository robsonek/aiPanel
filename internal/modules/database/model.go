package database

import "time"

// SiteDatabase represents one DB record associated with a site.
type SiteDatabase struct {
	ID        int64     `json:"id"`
	SiteID    int64     `json:"site_id"`
	DBName    string    `json:"db_name"`
	DBUser    string    `json:"db_user"`
	DBEngine  string    `json:"db_engine"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateDatabaseRequest contains payload for DB creation.
type CreateDatabaseRequest struct {
	SiteID   int64  `json:"site_id"`
	DBName   string `json:"db_name"`
	DBEngine string `json:"db_engine,omitempty"`
	Actor    string `json:"-"`
}

// CreateDatabaseResult includes one-time password for the new DB user.
type CreateDatabaseResult struct {
	Database SiteDatabase `json:"database"`
	Password string       `json:"password"`
}
