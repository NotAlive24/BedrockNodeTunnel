//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type backgroundConfig struct {
	Binary string
	Args   []string
}

func backgroundPaths() (string, string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "bedrocknode-tunnel", "background.json"),
		filepath.Join(dir, "systemd", "user", "bedrocknode-tunnel.service"), nil
}

func configureBackground(binary string, args []string) error {
	statePath, unitPath, err := backgroundPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(backgroundConfig{Binary: binary, Args: args})
	if err != nil {
		return err
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return err
	}

	parts := []string{strconv.Quote(binary)}
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	unit := "[Unit]\n" +
		"Description=Nethernet Bedrock Node Tunnel\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + strings.Join(parts, " ") + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"

	tmpPath := unitPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(unit), 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, unitPath)
}

func systemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("systemctl timed out")
	}
	if err != nil {
		return errors.New(strings.TrimSpace(string(output)) + ": " + err.Error())
	}
	return nil
}

func backgroundStatus() (bool, error) {
	systemctlPath, err := exec.LookPath("systemctl")
	if errors.Is(err, exec.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "is-active", "bedrocknode-tunnel.service")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false, errors.New("systemctl status check timed out")
	}
	if err == nil {
		switch strings.TrimSpace(string(output)) {
		case "active":
			return true, nil
		case "inactive", "failed":
			return false, nil
		default:
			return false, fmt.Errorf("unexpected systemd service state: %q", strings.TrimSpace(string(output)))
		}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// systemctl returns 3 for inactive and 4 for an unknown unit.
		if exitErr.ExitCode() == 3 || exitErr.ExitCode() == 4 {
			return false, nil
		}
		return false, fmt.Errorf("systemctl status check failed: %s", strings.TrimSpace(string(output)))
	}
	return false, err
}

func startBackground() error {
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	return systemctl("enable", "--now", "bedrocknode-tunnel.service")
}

func stopBackground() error {
	return systemctl("stop", "bedrocknode-tunnel.service")
}

func restartBackground() error {
	return systemctl("restart", "bedrocknode-tunnel.service")
}

func removeBackground() error {
	if err := systemctl("disable", "--now", "bedrocknode-tunnel.service"); err != nil {
		active, statusErr := backgroundStatus()
		if statusErr != nil {
			return fmt.Errorf("could not verify background service state after disable failed: %w", statusErr)
		}
		if active {
			return fmt.Errorf("could not stop active background service: %w", err)
		}
	}
	_, unitPath, err := backgroundPaths()
	if err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return systemctl("daemon-reload")
}

func backgroundSupported() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}
