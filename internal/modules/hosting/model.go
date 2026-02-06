package hosting

import "time"

// Site represents one hosted website record.
type Site struct {
	ID         int64     `json:"id"`
	Domain     string    `json:"domain"`
	RootDir    string    `json:"root_dir"`
	PHPVersion string    `json:"php_version"`
	SystemUser string    `json:"system_user"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateSiteRequest contains data needed to create a site.
type CreateSiteRequest struct {
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version"`
	Actor      string `json:"-"`
}
