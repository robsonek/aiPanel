package adapter

import "context"

// PHPFPM defines operations required to manage per-site FPM pools.
type PHPFPM interface {
	WritePool(ctx context.Context, site SiteConfig) error
	RemovePool(ctx context.Context, domain, phpVersion string) error
	Restart(ctx context.Context, phpVersion string) error
	ListVersions(ctx context.Context) ([]string, error)
}
