//go:build !linux && !windows

package backend

import "errors"

func backgroundSupported() bool { return false }
func backgroundInstalled() bool { return false }
func backgroundEnabled() bool { return false }
func backgroundActive() bool { return false }
func legacyBackgroundActive() bool { return false }
func legacyBackgroundEnabled() bool { return false }
func installBackground(Settings) error { return errors.New("unsupported platform") }
func updateBackground(Settings) error { return errors.New("unsupported platform") }
func startBackground() error { return errors.New("unsupported platform") }
func restartBackground() error { return errors.New("unsupported platform") }
func stopBackground() error { return errors.New("unsupported platform") }
func removeBackground() error { return errors.New("unsupported platform") }
