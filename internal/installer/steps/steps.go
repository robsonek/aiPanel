// Package steps contains ordered install steps (validate, deps, configure...).
package steps

const (
	Preflight      = "preflight"
	SystemUpdate   = "system_update"
	AddRepos       = "add_repositories"
	InstallPkgs    = "install_packages"
	PrepareDirs    = "prepare_dirs"
	CopyBinary     = "copy_binary"
	WriteConfig    = "write_config"
	CreateUser     = "create_service_user"
	InstallNginx   = "install_nginx"
	InitDatabases  = "init_databases"
	ConfigureNginx = "configure_nginx"
	ConfigurePHP   = "configure_phpfpm"
	WriteUnit      = "write_systemd_unit"
	StartPanel     = "start_panel_service"
	CreateAdmin    = "create_admin"
	Healthcheck    = "healthcheck"
)

// Ordered defines installer step execution sequence for phase 2.
var Ordered = []string{
	Preflight,
	SystemUpdate,
	AddRepos,
	InstallPkgs,
	PrepareDirs,
	CopyBinary,
	WriteConfig,
	CreateUser,
	InstallNginx,
	InitDatabases,
	ConfigureNginx,
	ConfigurePHP,
	WriteUnit,
	StartPanel,
	CreateAdmin,
	Healthcheck,
}
