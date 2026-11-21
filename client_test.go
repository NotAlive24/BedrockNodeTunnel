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
	if got, err := validateWebURL("http://127.0.0.1:8080"); err != nil || got != "http://127.0.0.1:8080" {
		t.Fatalf("loopback HTTP should be allowed for testing, got %q, %v", got, err)
	}
	for _, bad := range []string{
		"ftp://x", "bedrocknode.net", "https://", "",
		"http://8.8.8.8:8080", "https://user:pass@bedrocknode.net",
		"https://bedrocknode.net/?claim=secret", "https://bedrocknode.net/#fragment",
	} {
		if _, err := validateWebURL(bad); err == nil {
			t.Errorf("validateWebURL(%q) should fail", bad)
		}
	}
}

func TestValidateControlAddr(t *testing.T) {
	for _, valid := range []string{"51.222.142.179:7000", "custom.example:7000", "[::1]:7000"} {
		if _, err := validateControlAddr(valid); err != nil {
			t.Errorf("validateControlAddr(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "example.com", "host:0", "host:70000", "host:abc", "bad host:7000"} {
		if _, err := validateControlAddr(invalid); err == nil {
			t.Errorf("validateControlAddr(%q) should fail", invalid)
		}
	}
}

func TestSanitizeDiscoveredNodeRejectsUntrustedDestinations(t *testing.T) {
	defaultNode := defaultRegionNodes[0]
	if _, ok := sanitizeDiscoveredNode(RegionNode{
		Code: defaultNode.Code, Name: "Changed", WebURL: "https://evil.example",
		ControlAddr: defaultNode.ControlAddr,
	}); ok {
		t.Fatal("untrusted dashboard domain must be rejected")
	}
	if _, ok := sanitizeDiscoveredNode(RegionNode{
		Code: defaultNode.Code, Name: "Changed", WebURL: defaultNode.WebURL,
		ControlAddr: "203.0.113.77:7000",
	}); ok {
		t.Fatal("arbitrary control IP must be rejected")
	}
	if _, ok := sanitizeDiscoveredNode(RegionNode{
		Code: "UNKNOWN", Name: "Unknown", WebURL: defaultNode.WebURL,
		ControlAddr: defaultNode.ControlAddr,
	}); ok {
		t.Fatal("unknown region must be rejected")
	}
	accepted, ok := sanitizeDiscoveredNode(RegionNode{Code: defaultNode.Code, Name: "North America", WebURL: defaultNode.WebURL})
	if !ok || accepted.ControlAddr != defaultNode.ControlAddr {
		t.Fatalf("known region fallback not accepted: %+v, %v", accepted, ok)
	}
}

type shortWriter struct {
	bytes.Buffer
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return w.Buffer.Write(p)
}

func TestWriteFramedPacketHandlesShortWrites(t *testing.T) {
	var wire shortWriter
	payload := []byte("hello framed data")
	if err := writeFramedPacket(&wire, payload); err != nil {
		t.Fatalf("writeFramedPacket with short writer: %v", err)
	}
	got, err := readFramedPacket(&wire.Buffer)
	if err != nil {
		t.Fatalf("readFramedPacket: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("round trip = %q, want %q", got, payload)
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

func TestSplitOptionalNodeTokenOnlyAcceptsBuiltInRegions(t *testing.T) {
	control, token := splitOptionalNodeToken("UK|valid-token")
	if control != defaultRegionNodes[2].ControlAddr || token != "valid-token" {
		t.Fatalf("known region prefix = (%q, %q)", control, token)
	}

	control, token = splitOptionalNodeToken("evil.example:7000|valid-token")
	if control != "" || token != "evil.example:7000|valid-token" {
		t.Fatalf("custom endpoint prefix must not redirect token: (%q, %q)", control, token)
	}
}

func TestValidateLocalTCPTarget(t *testing.T) {
	for _, valid := range []string{"127.0.0.1:19132", "localhost:19132", "[::1]:19132"} {
		if err := validateLocalTCPTarget(valid); err != nil {
			t.Errorf("validateLocalTCPTarget(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "localhost", "host:0", "host:65536", "host name:19132"} {
		if err := validateLocalTCPTarget(invalid); err == nil {
			t.Errorf("validateLocalTCPTarget(%q) should fail", invalid)
		}
	}
}
