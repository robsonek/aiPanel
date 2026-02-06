package adapter

import "context"

// SiteConfig carries site-specific values used by system adapters.
type SiteConfig struct {
	Domain     string
	RootDir    string
	PHPVersion string
	SystemUser string
}

// Nginx defines operations required to manage per-site vhost config.
type Nginx interface {
	WriteVhost(ctx context.Context, site SiteConfig) error
	RemoveVhost(ctx context.Context, domain string) error
	TestConfig(ctx context.Context) error
	Reload(ctx context.Context) error
}
