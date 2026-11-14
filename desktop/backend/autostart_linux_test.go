//go:build linux

package backend

import (
	"strings"
	"testing"
)

func TestRenderAutostartEntry(t *testing.T) {
	entry := renderAutostartEntry("/home/test user/Nethernet Desktop", true)
	wantExec := "Exec=" + quoteDesktopExecArg("/home/test user/Nethernet Desktop") + " " + quoteDesktopExecArg("--background")
	for _, want := range []string{"[Desktop Entry]",wantExec,"Terminal=false","X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(entry, want) { t.Fatalf("autostart entry missing %q: %s", want, entry) }
	}
}
