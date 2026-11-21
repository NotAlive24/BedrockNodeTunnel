//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundStatusWithoutSystemctl(t *testing.T) {
	previousSystemdDir := systemdRunDirectory
	systemdRunDirectory = t.TempDir()
	defer func() { systemdRunDirectory = previousSystemdDir }()
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "bus"), nil, 0600); err != nil {
		t.Fatalf("create fake user bus: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	path := t.TempDir()
	t.Setenv("PATH", path)

	active, err := backgroundStatus()
	if err != nil {
		t.Fatalf("backgroundStatus without systemctl returned error: %v", err)
	}
	if active {
		t.Fatal("backgroundStatus without systemctl = active, want inactive")
	}
	if backgroundSupported() {
		t.Fatal("backgroundSupported without systemctl = true, want false")
	}
}

func TestBackgroundStatusParsesSystemctlState(t *testing.T) {
	previousSystemdDir := systemdRunDirectory
	systemdRunDirectory = t.TempDir()
	defer func() { systemdRunDirectory = previousSystemdDir }()
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "bus"), nil, 0600); err != nil {
		t.Fatalf("create fake user bus: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$BNT_SYSTEMD_STATUS\"\n" +
		"case \"$BNT_SYSTEMD_STATUS\" in\n" +
		"  active) exit 0 ;;\n" +
		"  inactive) exit 3 ;;\n" +
		"  unknown) exit 4 ;;\n" +
		"  *) echo 'Failed to connect to bus' >&2; exit 1 ;;\n" +
		"esac\n"
	scriptPath := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	t.Setenv("PATH", dir)

	t.Setenv("BNT_SYSTEMD_STATUS", "active")
	active, err := backgroundStatus()
	if err != nil || !active {
		t.Fatalf("active status = (%v, %v), want (true, nil)", active, err)
	}

	t.Setenv("BNT_SYSTEMD_STATUS", "inactive")
	active, err = backgroundStatus()
	if err != nil || active {
		t.Fatalf("inactive status = (%v, %v), want (false, nil)", active, err)
	}

	t.Setenv("BNT_SYSTEMD_STATUS", "broken")
	active, err = backgroundStatus()
	if err == nil || active {
		t.Fatalf("broken status = (%v, %v), want (false, error)", active, err)
	}
}
