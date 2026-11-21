//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLinuxUninstallHelperShellSyntax(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "uninstall.sh")
	if err := os.WriteFile(scriptPath, []byte(linuxUninstallScript), 0700); err != nil {
		t.Fatalf("write uninstall helper: %v", err)
	}
	if output, err := exec.Command("sh", "-n", scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("Linux uninstall helper has invalid shell syntax: %v\n%s", err, output)
	}
}
