package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIsValidToken(t *testing.T) {
	cases := map[string]bool{
		"abc123":                              true,
		strings.Repeat("a", maxTokenLength):   true,
		strings.Repeat("a", maxTokenLength+1): false,
		"":                                    false,
		"has space":                           false,
		"tab\there":                           false,
		"caf\u00e9":                           false,
	}
	for in, want := range cases {
		if got := isValidToken(in); got != want {
			t.Errorf("isValidToken(len=%d) = %v, want %v", len(in), got, want)
		}
	}
}

func TestReadToken(t *testing.T) {
	long := strings.Repeat("A", 300)
	if tok, ok := readToken(strings.NewReader(long + "\n")); !ok || tok != long {
		t.Fatalf("300-char token not read intact (ok=%v len=%d)", ok, len(tok))
	}
	if _, ok := readToken(strings.NewReader(strings.Repeat("A", maxTokenLength+1))); ok {
		t.Fatal("oversized token must be rejected, not truncated")
	}
	if _, ok := readToken(strings.NewReader("bad token")); ok {
		t.Fatal("token with a space must be rejected")
	}
}

func TestTokenPathOrderAndRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", home)

	paths := candidateTokenPaths()
	if len(paths) < 2 || paths[len(paths)-1] != tokenSaveFile {
		t.Fatalf("legacy path must come last: %v", paths)
	}

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(t.TempDir())

	if err := os.WriteFile(tokenSaveFile, []byte("stale-cwd"), 0600); err != nil {
		t.Fatal(err)
	}
	saved := saveToken("fresh-config")
	if saved == "" || saved == tokenSaveFile {
		t.Fatalf("token should be saved in the config dir, got %q", saved)
	}
	if got, from := loadSavedToken(); got != "fresh-config" || from != saved {
		t.Fatalf("config token must win over stale cwd token, got %q from %q", got, from)
	}
	clearSavedTokens()
	if got, _ := loadSavedToken(); got != "" {
		t.Fatalf("tokens not cleared, still got %q", got)
	}
}

func TestValidateWebURL(t *testing.T) {
	if got, err := validateWebURL(" https://bedrocknode.net/ "); err != nil || got != "https://bedrocknode.net" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"ftp://x", "bedrocknode.net", "https://", ""} {
		if _, err := validateWebURL(bad); err == nil {
			t.Errorf("validateWebURL(%q) should fail", bad)
		}
	}
}

func TestFramedPacketRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello")
	if err := writeFramedPacket(&buf, payload); err != nil {
		t.Fatal(err)
	}
	got, err := readFramedPacket(&buf)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := writeFramedPacket(&buf, make([]byte, 70000)); err == nil {
		t.Fatal("oversized packet must be rejected")
	}
}
