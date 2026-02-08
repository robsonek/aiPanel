package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// RuntimeSourceLock represents pinned upstream source metadata for runtime components.
type RuntimeSourceLock struct {
	SchemaVersion int                           `json:"schema_version"`
	Channels      map[string]RuntimeChannelLock `json:"channels"`
}

// RuntimeChannelLock groups component metadata under a release channel.
type RuntimeChannelLock map[string]RuntimeComponentLock

// RuntimeComponentLock describes one pinned component source definition.
type RuntimeComponentLock struct {
	Version              string                 `json:"version"`
	SourceURL            string                 `json:"source_url"`
	SourceSHA256         string                 `json:"source_sha256"`
	SignatureURL         string                 `json:"signature_url"`
	PublicKeyFingerprint string                 `json:"public_key_fingerprint"`
	Build                RuntimeBuildSpec       `json:"build,omitempty"`
	Systemd              RuntimeSystemdUnitSpec `json:"systemd,omitempty"`
}

// RuntimeBuildSpec declares source build commands for a runtime component.
type RuntimeBuildSpec struct {
	// Commands run in order from the extracted source directory.
	// Placeholders supported: {{runtime_dir}}, {{component}}, {{version}}, {{install_dir}}.
	Commands []string `json:"commands,omitempty"`
}

// RuntimeSystemdUnitSpec declares how to run a runtime component through systemd.
type RuntimeSystemdUnitSpec struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Type             string   `json:"type,omitempty"`
	User             string   `json:"user,omitempty"`
	Group            string   `json:"group,omitempty"`
	ExecStart        string   `json:"exec_start"`
	ExecReload       string   `json:"exec_reload,omitempty"`
	ExecStop         string   `json:"exec_stop,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	After            []string `json:"after,omitempty"`
	Wants            []string `json:"wants,omitempty"`
}

// LoadRuntimeSourceLock loads and validates runtime source lock metadata from a JSON file.
func LoadRuntimeSourceLock(path string) (*RuntimeSourceLock, error) {
	// Installer controls the lockfile path in runtime options.
	//nolint:gosec // G304
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime lock file: %w", err)
	}
	var lock RuntimeSourceLock
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("decode runtime lock file: %w", err)
	}
	if err := lock.Validate(); err != nil {
		return nil, err
	}
	return &lock, nil
}

// Validate enforces minimum schema and field-level correctness.
func (l RuntimeSourceLock) Validate() error {
	if l.SchemaVersion != 1 {
		return fmt.Errorf("unsupported runtime lock schema version: %d", l.SchemaVersion)
	}
	if len(l.Channels) == 0 {
		return fmt.Errorf("runtime lock has no channels")
	}

	channelNames := make([]string, 0, len(l.Channels))
	for name := range l.Channels {
		channelNames = append(channelNames, name)
	}
	sort.Strings(channelNames)

	for _, channelName := range channelNames {
		channel := l.Channels[channelName]
		if len(channel) == 0 {
			return fmt.Errorf("runtime lock channel %s has no components", channelName)
		}
		for componentName, component := range channel {
			if err := validateRuntimeComponentLock(channelName, componentName, component); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRuntimeComponentLock(channel, name string, component RuntimeComponentLock) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("runtime lock channel %s contains empty component name", channel)
	}
	if strings.TrimSpace(component.Version) == "" {
		return fmt.Errorf("runtime lock component %s/%s is missing version", channel, name)
	}
	if strings.TrimSpace(component.SourceURL) == "" {
		return fmt.Errorf("runtime lock component %s/%s is missing source_url", channel, name)
	}
	if !isValidSHA256(component.SourceSHA256) {
		return fmt.Errorf("runtime lock component %s/%s has invalid source_sha256", channel, name)
	}
	signatureURL := strings.TrimSpace(component.SignatureURL)
	signatureFP := strings.TrimSpace(component.PublicKeyFingerprint)
	if (signatureURL == "") != (signatureFP == "") {
		if signatureURL == "" {
			return fmt.Errorf("runtime lock component %s/%s is missing signature_url", channel, name)
		}
		return fmt.Errorf("runtime lock component %s/%s is missing public_key_fingerprint", channel, name)
	}
	if err := validateRuntimeBuildSpec(channel, name, component.Build); err != nil {
		return err
	}
	if err := validateRuntimeSystemdUnit(channel, name, component.Systemd); err != nil {
		return err
	}
	return nil
}

func validateRuntimeBuildSpec(channel, component string, build RuntimeBuildSpec) error {
	if len(build.Commands) == 0 {
		return nil
	}
	for idx, cmd := range build.Commands {
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf(
				"runtime lock component %s/%s build.commands[%d] is empty",
				channel,
				component,
				idx,
			)
		}
	}
	return nil
}

func validateRuntimeSystemdUnit(channel, component string, unit RuntimeSystemdUnitSpec) error {
	if strings.TrimSpace(unit.Name) == "" &&
		strings.TrimSpace(unit.ExecStart) == "" &&
		strings.TrimSpace(unit.Type) == "" &&
		strings.TrimSpace(unit.ExecReload) == "" &&
		strings.TrimSpace(unit.ExecStop) == "" &&
		strings.TrimSpace(unit.WorkingDirectory) == "" &&
		len(unit.After) == 0 &&
		len(unit.Wants) == 0 {
		return nil
	}
	if strings.TrimSpace(unit.Name) == "" {
		return fmt.Errorf("runtime lock component %s/%s systemd.name is required when systemd block is set", channel, component)
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(unit.Name)), ".service") {
		return fmt.Errorf("runtime lock component %s/%s systemd.name must end with .service", channel, component)
	}
	if strings.TrimSpace(unit.ExecStart) == "" {
		return fmt.Errorf("runtime lock component %s/%s systemd.exec_start is required when systemd block is set", channel, component)
	}
	return nil
}

func isValidSHA256(value string) bool {
	v := strings.TrimSpace(value)
	if len(v) != 64 {
		return false
	}
	for _, ch := range v {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}
