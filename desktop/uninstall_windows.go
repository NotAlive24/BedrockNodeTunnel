//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const windowsUninstallScript = `param(
    [int]$ParentPid,
    [string]$Uninstaller,
    [string]$ConfigDir,
    [switch]$PurgeData
)

$ErrorActionPreference = "Stop"

# Wait until the desktop process closes so Windows can remove its executable.
for ($attempt = 0; $attempt -lt 240; $attempt++) {
    if (-not (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue)) {
        break
    }
    Start-Sleep -Milliseconds 250
}
if (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue) {
    exit 2
}

if (-not (Test-Path -LiteralPath $Uninstaller)) {
    exit 3
}

$arguments = @("/S")
if ($PurgeData) {
    $arguments += "/PURGE"
}
$uninstall = Start-Process -FilePath $Uninstaller -ArgumentList $arguments -Wait -PassThru
if ($uninstall.ExitCode -ne 0) {
    exit $uninstall.ExitCode
}

if ($PurgeData -and (Test-Path -LiteralPath $ConfigDir)) {
    Remove-Item -LiteralPath $ConfigDir -Recurse -Force
}

$scriptPath = $PSCommandPath
if ($scriptPath -and (Test-Path -LiteralPath $scriptPath)) {
    Remove-Item -LiteralPath $scriptPath -Force -ErrorAction SilentlyContinue
}
`

func launchPlatformUninstaller(parentPID int, configDir string, purgeData bool) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate installed desktop executable: %w", err)
	}
	uninstaller := filepath.Join(filepath.Dir(executable), "Uninstall.exe")
	if _, err := os.Stat(uninstaller); err != nil {
		return errors.New("the NSIS uninstaller was not found; this may be a portable build, so remove its folder manually")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		return errors.New("Windows PowerShell was not found; run Uninstall.exe from the application folder")
	}

	script, err := os.CreateTemp("", "Nethernet-Uninstall-*.ps1")
	if err != nil {
		return fmt.Errorf("create uninstall helper: %w", err)
	}
	scriptPath := script.Name()
	cleanup := func() { _ = os.Remove(scriptPath) }
	if err := script.Chmod(0600); err != nil {
		_ = script.Close()
		cleanup()
		return err
	}
	if _, err := script.WriteString(windowsUninstallScript); err != nil {
		_ = script.Close()
		cleanup()
		return err
	}
	if err := script.Close(); err != nil {
		cleanup()
		return err
	}

	args := []string{
		"-NoProfile",
		"-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-ParentPid", strconv.Itoa(parentPID),
		"-Uninstaller", uninstaller,
		"-ConfigDir", filepath.Clean(configDir),
	}
	if purgeData {
		args = append(args, "-PurgeData")
	}
	cmd := exec.Command(powershell, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
	if err := cmd.Start(); err != nil {
		cleanup()
		return fmt.Errorf("start Windows uninstaller helper: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
