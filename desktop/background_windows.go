//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const taskName = "NethernetBedrockNodeTunnel"

type backgroundConfig struct {
	Binary string
	Args   []string
}

func backgroundPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bedrocknode-tunnel", "background.json"), nil
}

func configureBackground(binary string, args []string) error {
	configPath, err := backgroundPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(backgroundConfig{Binary: binary, Args: args})
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return err
	}

	_, err = taskCommand()
	return err
}

func taskCommand() (string, error) {
	configPath, err := backgroundPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var config backgroundConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	args := make([]string, 0, len(config.Args)+1)
	args = append(args, quoteWindowsArg(config.Binary))
	for _, arg := range config.Args {
		args = append(args, quoteWindowsArg(arg))
	}
	return strings.Join(args, " "), nil
}

// quoteWindowsArg follows the CommandLineToArgvW quoting rules.
func quoteWindowsArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			b.WriteString(strings.Repeat("\\", backslashes))
			b.WriteRune(r)
			backslashes = 0
		}
	}
	b.WriteString(strings.Repeat("\\", backslashes*2))
	b.WriteByte('"')
	return b.String()
}

func task(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks.exe", args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("Windows Task Scheduler command timed out")
	}
	if err != nil {
		return fmt.Errorf("Task Scheduler command failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func backgroundStatus() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks.exe", "/Query", "/TN", taskName, "/FO", "LIST", "/V")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false, errors.New("scheduled-task status check timed out")
	}
	if err != nil {
		lower := strings.ToLower(string(output))
		if strings.Contains(lower, "cannot find the file specified") ||
			strings.Contains(lower, "cannot find the task") {
			return false, nil
		}
		return false, fmt.Errorf("could not query background task: %s: %w", strings.TrimSpace(string(output)), err)
	}

	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "running") {
		return true, nil
	}
	if strings.Contains(lower, "ready") || strings.Contains(lower, "disabled") {
		return false, nil
	}
	return false, fmt.Errorf("could not determine background task state from Task Scheduler response")
}

func startBackground() error {
	commandLine, err := taskCommand()
	if err != nil {
		return err
	}
	if err := task("/Create", "/SC", "ONLOGON", "/TN", taskName, "/TR", commandLine, "/F"); err != nil {
		return err
	}
	return task("/Run", "/TN", taskName)
}

func stopBackground() error {
	return task("/End", "/TN", taskName)
}

func restartBackground() error {
	if err := stopBackground(); err != nil {
		return err
	}
	return task("/Run", "/TN", taskName)
}

func removeBackground() error {
	if err := stopBackground(); err != nil {
		active, statusErr := backgroundStatus()
		if statusErr != nil {
			return fmt.Errorf("could not verify scheduled-task state after stop failed: %w", statusErr)
		}
		if active {
			return fmt.Errorf("could not stop active background task: %w", err)
		}
	}
	if err := task("/Delete", "/TN", taskName, "/F"); err != nil {
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "cannot find the file specified") &&
			!strings.Contains(lower, "cannot find the task") {
			return err
		}
	}
	configPath, err := backgroundPath()
	if err != nil {
		return err
	}
	if err := os.Remove(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func backgroundSupported() bool {
	_, err := exec.LookPath("schtasks.exe")
	return err == nil
}
