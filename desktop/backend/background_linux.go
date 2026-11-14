//go:build linux

package backend

import (
	"errors"
	"os"
	"path/filepath"
)

func backgroundSupported() bool { return hasSystemctl() }
func backgroundInstalled() bool { return fileExists(unitPath()) }
func backgroundEnabled() bool { return unitEnabled(unitName) }
func backgroundActive() bool { return unitActive(unitName) }
func legacyBackgroundActive() bool { return unitActive(legacyUnit) }
func legacyBackgroundEnabled() bool { return unitEnabled(legacyUnit) }

func writeSystemdUnit(v Settings) error {
	bin, err := resolve(v)
	if err != nil { return err }
	path := unitPath()
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil { return err }
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, []byte(renderUnit(bin, clientArgs(v))), 0600); err != nil { return err }
	return os.Rename(tmp, path)
}
func installBackground(v Settings) error {
	if err := writeSystemdUnit(v); err != nil { return err }
	_, err := systemctl("daemon-reload")
	return err
}
func updateBackground(v Settings) error { return installBackground(v) }
func startBackground() error { _, err := systemctl("enable", "--now", unitName); return err }
func restartBackground() error { _, err := systemctl("restart", unitName); return err }
func stopBackground() error {
	_, err := systemctl("stop", unitName)
	if err != nil && !unitActive(unitName) { return nil }
	return err
}
func removeBackground() error {
	_, err := systemctl("disable", "--now", unitName)
	if err != nil && unitActive(unitName) { return err }
	if err := os.Remove(unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) { return err }
	_, err = systemctl("daemon-reload")
	return err
}
