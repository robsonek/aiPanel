package installer

import (
	"strings"
	"testing"
)

func TestOptionsValidate(t *testing.T) {
	t.Run("valid source-build defaults", func(t *testing.T) {
		opts := DefaultOptions()
		if err := opts.validate(); err != nil {
			t.Fatalf("expected valid options, got %v", err)
		}
	})

	t.Run("invalid install mode", func(t *testing.T) {
		opts := DefaultOptions()
		opts.InstallMode = "unknown"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "invalid install mode") {
			t.Fatalf("expected invalid install mode error, got %v", err)
		}
	})

	t.Run("invalid runtime channel", func(t *testing.T) {
		opts := DefaultOptions()
		opts.RuntimeChannel = "nightly"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "invalid runtime channel") {
			t.Fatalf("expected invalid runtime channel error, got %v", err)
		}
	})

	t.Run("source-build mode validates runtime lock dependency", func(t *testing.T) {
		opts := DefaultOptions()
		opts.InstallMode = InstallModeSourceBuild
		opts.RuntimeLockPath = ""
		opts.RuntimeManifestURL = ""
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "requires runtime lock path or runtime manifest URL") {
			t.Fatalf("expected source-build dependency validation error, got %v", err)
		}
	})
}

func TestOptionsWithDefaults(t *testing.T) {
	var opts Options
	opts = opts.withDefaults()

	if opts.InstallMode != InstallModeSourceBuild {
		t.Fatalf("expected install mode %q, got %q", InstallModeSourceBuild, opts.InstallMode)
	}
	if opts.RuntimeChannel != RuntimeChannelStable {
		t.Fatalf("expected runtime channel %q, got %q", RuntimeChannelStable, opts.RuntimeChannel)
	}
	if opts.RuntimeLockPath == "" {
		t.Fatal("expected runtime lock path default to be set")
	}
	if opts.RuntimeInstallDir == "" {
		t.Fatal("expected runtime install dir default to be set")
	}
}
