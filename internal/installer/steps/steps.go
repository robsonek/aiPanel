// Package steps contains ordered install steps (validate, deps, configure...).
package steps

const (
	Preflight    = "preflight"
	PrepareDirs  = "prepare_dirs"
	CopyBinary   = "copy_binary"
	WriteConfig  = "write_config"
	CreateUser   = "create_service_user"
	InstallNginx = "install_nginx"
	WriteUnit    = "write_systemd_unit"
	StartPanel   = "start_panel_service"
	Healthcheck  = "healthcheck"
)

// Ordered defines installer step execution sequence for phase 1.
var Ordered = []string{
	Preflight,
	PrepareDirs,
	CopyBinary,
	WriteConfig,
	CreateUser,
	InstallNginx,
	WriteUnit,
	StartPanel,
	Healthcheck,
}
