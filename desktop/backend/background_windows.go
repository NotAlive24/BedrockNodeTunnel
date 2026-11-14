//go:build windows

package backend

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runSchtasks(args ...string) (string, error) {
	exe, err := exec.LookPath("schtasks.exe")
	if err != nil {
		root := os.Getenv("SystemRoot")
		if root == "" { return "", fmt.Errorf("schtasks.exe was not found") }
		exe = filepath.Join(root, "System32", "schtasks.exe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("schtasks %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func backgroundSupported() bool { return os.Getenv("SystemRoot") != "" }
func backgroundInstalled() bool { _, err := runSchtasks("/Query", "/TN", windowsTaskName); return err == nil }
func backgroundEnabled() bool { return backgroundInstalled() }
func backgroundActive() bool {
	out, err := runSchtasks("/Query", "/TN", windowsTaskName, "/FO", "CSV", "/NH")
	if err != nil { return false }
	rows, err := csv.NewReader(strings.NewReader(strings.TrimSpace(out))).ReadAll()
	if err != nil || len(rows) == 0 { return false }
	for _, field := range rows[0] {
		if strings.EqualFold(strings.TrimSpace(field), "Running") { return true }
	}
	return false
}
func legacyBackgroundActive() bool { return false }
func legacyBackgroundEnabled() bool { return false }

// Quote an argument using the Windows command-line parsing rules used by
// CreateProcess. Task Scheduler stores the resulting command line as one action.
func quoteWindowsArg(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\v\"") { return value }
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		if r == '\\' { backslashes++; continue }
		if r == '"' {
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
			continue
		}
		b.WriteString(strings.Repeat("\\", backslashes))
		backslashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat("\\", backslashes*2))
	b.WriteByte('"')
	return b.String()
}

func windowsTaskAction(v Settings) (string, error) {
	bin, err := resolve(v)
	if err != nil { return "", err }
	args := append([]string{bin}, clientArgs(v)...)
	for i := range args { args[i] = quoteWindowsArg(args[i]) }
	return strings.Join(args, " "), nil
}
func installBackground(v Settings) error {
	action, err := windowsTaskAction(v)
	if err != nil { return err }
	_, err = runSchtasks("/Create", "/TN", windowsTaskName, "/SC", "ONLOGON", "/TR", action, "/RL", "LIMITED", "/F")
	return err
}
func updateBackground(v Settings) error { return installBackground(v) }
func startBackground() error { _, err := runSchtasks("/Run", "/TN", windowsTaskName); return err }
func restartBackground() error {
	if backgroundActive() {
		if _, err := runSchtasks("/End", "/TN", windowsTaskName); err != nil { return err }
	}
	return startBackground()
}
func stopBackground() error {
	if !backgroundActive() { return nil }
	_, err := runSchtasks("/End", "/TN", windowsTaskName)
	return err
}
func removeBackground() error {
	if !backgroundInstalled() { return nil }
	_, err := runSchtasks("/Delete", "/TN", windowsTaskName, "/F")
	return err
}
