//go:build windows

package backend

import "testing"

func TestWindowsQuoteArg(t *testing.T) {
	got := quoteWindowsArg("C:\\Program Files\\Nethernet\\nethernet-client.exe")
	want := "\"C:\\Program Files\\Nethernet\\nethernet-client.exe\""
	if got != want { t.Fatalf("got %q, want %q", got, want) }
}
