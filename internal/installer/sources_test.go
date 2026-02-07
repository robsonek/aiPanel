package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuntimeSourceLock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lock.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "https://nginx.org/download/nginx-1.27.4.tar.gz",
        "source_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	lock, err := LoadRuntimeSourceLock(path)
	if err != nil {
		t.Fatalf("load lock file: %v", err)
	}
	if lock.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", lock.SchemaVersion)
	}
	component := lock.Channels[RuntimeChannelStable]["nginx"]
	if component.Version != "1.27.4" {
		t.Fatalf("expected nginx version 1.27.4, got %s", component.Version)
	}
}

func TestLoadRuntimeSourceLock_InvalidChecksum(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lock-invalid.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "https://nginx.org/download/nginx-1.27.4.tar.gz",
        "source_sha256": "not-a-checksum",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	_, err := LoadRuntimeSourceLock(path)
	if err == nil {
		t.Fatal("expected invalid checksum error")
	}
	if !strings.Contains(err.Error(), "invalid source_sha256") {
		t.Fatalf("expected source_sha256 validation error, got: %v", err)
	}
}

func TestLoadRuntimeSourceLock_InvalidSystemdBlock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lock-invalid-systemd.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "https://nginx.org/download/nginx-1.27.4.tar.gz",
        "source_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62",
        "systemd": {
          "name": "aipanel-runtime-nginx.service"
        }
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	_, err := LoadRuntimeSourceLock(path)
	if err == nil {
		t.Fatal("expected invalid systemd block error")
	}
	if !strings.Contains(err.Error(), "systemd.exec_start is required") {
		t.Fatalf("expected systemd exec_start validation error, got: %v", err)
	}
}

func TestLoadRuntimeSourceLock_InvalidBuildCommands(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lock-invalid-build.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "https://nginx.org/download/nginx-1.27.4.tar.gz",
        "source_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62",
        "build": {
          "commands": ["", "make -j$(nproc)"]
        }
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	_, err := LoadRuntimeSourceLock(path)
	if err == nil {
		t.Fatal("expected invalid build command error")
	}
	if !strings.Contains(err.Error(), "build.commands[0] is empty") {
		t.Fatalf("expected build.commands validation error, got: %v", err)
	}
}
