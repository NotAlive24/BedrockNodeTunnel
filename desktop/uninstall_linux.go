//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const linuxUninstallScript = `#!/bin/sh
parent_pid="$1"
manager="$2"
purge_data="$3"
config_dir="$4"

# Wait until the desktop process releases its installed executable and
# configuration files before invoking the native package manager.
attempt=0
while kill -0 "$parent_pid" 2>/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 240 ]; then
    exit 2
  fi
  sleep 0.25
done

case "$manager" in
  pacman)
    pkexec pacman -Rns --noconfirm nethernet-desktop
    result=$?
    ;;
  apt)
    pkexec apt-get remove -y nethernet-desktop
    result=$?
    ;;
  *)
    result=2
    ;;
esac

if [ "$result" -eq 0 ] && [ "$purge_data" = "true" ]; then
  rm -rf -- "$config_dir"
fi

if [ "$result" -ne 0 ] && command -v notify-send >/dev/null 2>&1; then
  notify-send "Nethernet Desktop" "Uninstall did not complete. Check your package manager and polkit permissions."
fi

rm -f -- "$0"
exit "$result"
`

func linuxPackageManager() (string, error) {
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		if _, err := exec.LookPath("pacman"); err != nil {
			return "", errors.New("pacman was not found; remove the nethernet-desktop package manually")
		}
		if _, err := exec.Command("pacman", "-Q", "nethernet-desktop").Output(); err != nil {
			return "", errors.New("Nethernet Desktop is not installed as an Arch package; portable copies must be removed manually")
		}
		return "pacman", nil
	}
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		if _, err := exec.LookPath("apt-get"); err != nil {
			return "", errors.New("apt-get was not found; remove the nethernet-desktop package manually")
		}
		dpkg, err := exec.LookPath("dpkg-query")
		if err != nil {
			return "", errors.New("dpkg-query was not found; remove the nethernet-desktop package manually")
		}
		status, err := exec.Command(dpkg, "-W", "-f=${db:Status-Status}", "nethernet-desktop").Output()
		if err != nil || strings.TrimSpace(string(status)) != "installed" {
			return "", errors.New("Nethernet Desktop is not installed as a Debian package; portable copies must be removed manually")
		}
		return "apt", nil
	}
	return "", errors.New("automatic uninstall is supported for Arch and Debian/Ubuntu packages; remove the application with your distribution's package manager")
}

func launchPlatformUninstaller(parentPID int, configDir string, purgeData bool) error {
	manager, err := linuxPackageManager()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return errors.New("polkit (pkexec) is required for the uninstall confirmation dialog; remove nethernet-desktop with your package manager")
	}

	script, err := os.CreateTemp("", "nethernet-desktop-uninstall-*.sh")
	if err != nil {
		return fmt.Errorf("create uninstall helper: %w", err)
	}
	scriptPath := script.Name()
	cleanup := func() { _ = os.Remove(scriptPath) }

	if err := script.Chmod(0700); err != nil {
		_ = script.Close()
		cleanup()
		return err
	}
	if _, err := script.WriteString(linuxUninstallScript); err != nil {
		_ = script.Close()
		cleanup()
		return err
	}
	if err := script.Close(); err != nil {
		cleanup()
		return err
	}

	cmd := exec.Command("sh", scriptPath,
		strconv.Itoa(parentPID),
		manager,
		strconv.FormatBool(purgeData),
		filepath.Clean(configDir),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		cleanup()
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	if err := cmd.Start(); err != nil {
		_ = null.Close()
		cleanup()
		return fmt.Errorf("start native uninstaller: %w", err)
	}
	_ = null.Close()
	go func() { _ = cmd.Wait() }()
	return nil
}
