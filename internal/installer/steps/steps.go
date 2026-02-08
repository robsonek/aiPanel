// Package steps contains ordered install steps (validate, deps, configure...).
package steps

const (
	Preflight         = "preflight"
	SystemUpdate      = "system_update"
	AddRepos          = "add_repositories"
	InstallPkgs       = "install_packages"
	PrepareDirs       = "prepare_dirs"
	InstallRuntime    = "install_runtime"
	ActivateRuntime   = "activate_runtime_services"
	CopyBinary        = "copy_binary"
	WriteConfig       = "write_config"
	CreateUser        = "create_service_user"
	InstallNginx      = "install_nginx"
	InitDatabases     = "init_databases"
	ConfigureNginx    = "configure_nginx"
	ConfigureTLS      = "configure_tls"
	ConfigurePHP      = "configure_phpfpm"
	InstallPHPMyAdmin = "install_phpmyadmin"
	InstallPGAdmin    = "install_pgadmin"
	WriteUnit         = "write_systemd_unit"
	StartPanel        = "start_panel_service"
	CreateAdmin       = "create_admin"
	Healthcheck       = "healthcheck"
)

// Ordered defines installer step execution sequence for phase 2.
var Ordered = []string{
	Preflight,
	SystemUpdate,
	AddRepos,
	InstallPkgs,
	PrepareDirs,
	InstallRuntime,
	ActivateRuntime,
	CopyBinary,
	WriteConfig,
	CreateUser,
	InstallNginx,
	InitDatabases,
	ConfigureNginx,
	ConfigureTLS,
	ConfigurePHP,
	InstallPHPMyAdmin,
	InstallPGAdmin,
	WriteUnit,
	StartPanel,
	CreateAdmin,
	Healthcheck,
}
