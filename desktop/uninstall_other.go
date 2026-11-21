//go:build !linux && !windows

package main

import "errors"

func launchPlatformUninstaller(parentPID int, configDir string, purgeData bool) error {
	return errors.New("automatic uninstall is supported by the Linux and Windows desktop installers; remove this application using the method you used to install it")
}
