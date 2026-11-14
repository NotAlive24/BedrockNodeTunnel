//go:build !linux && !windows

package backend

import "errors"
func configureAutostart(enabled,startMinimized bool) error { return errors.New("desktop autostart is currently supported on Linux and Windows") }
