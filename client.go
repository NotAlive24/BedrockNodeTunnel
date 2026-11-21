package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
)

const (
	StreamTypeHTTP       = 0x01
	StreamTypeUDP        = 0x02
	StreamTypeTCP        = 0x03
	StreamTypeTCPMapped  = 0x04 // [0x04][2B localTCPPort] for multi-server tunnels
	RawUDPMagicHeartbeat = 0x01 // [0x01][token bytes] -> Server replies [0x01, 0x01]
	RawUDPMagicData      = 0x02 // [0x02][2B publicUDPPort][4B playerIPv4][2B playerPort][payload]
	tokenSaveFile        = "bedrocknode-token.txt"
	nodeSaveFile         = "bedrocknode-node.txt"
	maxTokenLength       = 4096
	handshakeHint        = 5
	maxRawPlayerSessions = 128
	maxUDPPacketPayload  = 65507
	tcpSocketBufferBytes = 512 * 1024
	udpSocketBufferBytes = 128 * 1024
	rawUDPSocketBufferBytes = 512 * 1024
)

var version = "v2.0.0"

var errInvalidToken = errors.New("invalid auth token rejected by server")

type RegionNode struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	WebURL      string `json:"webUrl"`
	ControlAddr string `json:"controlAddr"`
}

var defaultRegionNodes = []RegionNode{
	{
		Code:        "US",
		Name:        "North America (US East Hub)",
		WebURL:      "https://bedrocknode.net",
		ControlAddr: "51.222.142.179:7000",
	},
	{
		Code:        "TX",
		Name:        "North America (US Texas Hub)",
		WebURL:      "https://texas.bedrocknode.net",
		ControlAddr: "216.128.129.60:7000",
	},
	{
		Code:        "UK",
		Name:        "Europe (London UK Hub)",
		WebURL:      "https://uk.bedrocknode.net",
		ControlAddr: "104.238.173.37:7000",
	},
	{
		Code:        "BR",
		Name:        "South America (Brazil Hub)",
		WebURL:      "https://southa.bedrocknode.net",
		ControlAddr: "216.128.170.42:7000",
	},
	{
		Code:        "ASIA",
		Name:        "Asia-Pacific (Singapore Hub)",
		WebURL:      "https://asia.bedrocknode.net",
		ControlAddr: "45.32.121.108:7000",
	},
}

type rawPlayerKey struct {
	pubPort    uint16
	playerIP   [4]byte
	playerPort uint16
}

type candConn struct {
	label string
	conn  *net.UDPConn
}

type rawLocalSession struct {
	conns      []candConn
	winnerIdx  atomic.Int32
	lastActive atomic.Int64
}

func (s *rawLocalSession) Close() {
	for _, c := range s.conns {
		if c.conn != nil {
			_ = c.conn.Close()
		}
	}
}

func main() {
	var (
		nodeFlag        string
		controlAddr     string
		webUrl          string
		token           string
		localSignaling  string
		localUDPIP      string
		localUDPDefault int
		resetToken      bool
		showVersion     bool
	)

	flag.StringVar(&nodeFlag, "node", "", "Region code (US, TX, UK, BR, ASIA) or custom VPS domain/IP")
	flag.StringVar(&controlAddr, "control", "", "Bedrock Node Tunnel VPS control address (auto-detected across regions if empty)")
	flag.StringVar(&webUrl, "web", "https://bedrocknode.net", "Bedrock Node Tunnel Web Dashboard URL")
	flag.StringVar(&token, "token", "", "Tunnel token (leave empty for automatic browser claim)")
	flag.StringVar(&localSignaling, "local-http", "127.0.0.1:19132", "Local Bedrock TCP target on Home PC")
	flag.StringVar(&localUDPIP, "local-udp-ip", "", "Local LAN IP for Bedrock UDP (auto-detected if empty)")
	flag.IntVar(&localUDPDefault, "local-udp-port", 47334, "Fallback local UDP port on Home PC")
	flag.BoolVar(&resetToken, "reset-token", false, "Delete the saved tunnel token and claim a new one")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return
	}

	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	token = strings.TrimSpace(token)
	if token != "" && !isValidToken(token) {
		log.Fatalf("[Bedrock Node Tunnel] Invalid -token value: must be 1-%d printable ASCII characters without spaces", maxTokenLength)
	}
	if resetToken {
		clearSavedTokens()
	}

	cleanWebURL, err := validateWebURL(webUrl)
	if err != nil {
		log.Fatalf("[Bedrock Node Tunnel] Invalid web URL: %v", err)
	}

	pollWebOverride := ""

	// Automatically pair -node, -web, and -control so partial flags never mismatch regions
	if nodeFlag != "" {
		n := resolveNodeInput(nodeFlag)
		if !explicitFlags["control"] {
			controlAddr = n.ControlAddr
		}
		if !explicitFlags["web"] && n.WebURL != "" {
			cleanWebURL = n.WebURL
		}
		pollWebOverride = cleanWebURL
	} else if explicitFlags["control"] && !explicitFlags["web"] {
		n := resolveNodeInput(controlAddr)
		controlAddr = n.ControlAddr
		if n.WebURL != "" {
			cleanWebURL = n.WebURL
		}
		pollWebOverride = cleanWebURL
	} else if explicitFlags["web"] && !explicitFlags["control"] {
		n := resolveNodeInput(cleanWebURL)
		controlAddr = n.ControlAddr
		pollWebOverride = cleanWebURL
	} else if explicitFlags["web"] && explicitFlags["control"] {
		pollWebOverride = cleanWebURL
	}

	if controlAddr != "" {
		controlAddr, err = validateControlAddr(controlAddr)
		if err != nil {
			log.Fatalf("[Bedrock Node Tunnel] Invalid control address: %v", err)
		}
	}
	if pollWebOverride != "" {
		pollWebOverride, err = validateWebURL(pollWebOverride)
		if err != nil {
			log.Fatalf("[Bedrock Node Tunnel] Invalid claim dashboard URL: %v", err)
		}
	}
	if err := validateLocalTCPTarget(localSignaling); err != nil {
		log.Fatalf("[Bedrock Node Tunnel] Invalid local TCP target: %v", err)
	}
	if localUDPDefault < 1 || localUDPDefault > 65535 {
		log.Fatalf("[Bedrock Node Tunnel] Local UDP fallback port must be between 1 and 65535")
	}
	if localUDPIP != "" && net.ParseIP(localUDPIP) == nil {
		log.Fatalf("[Bedrock Node Tunnel] Local UDP IP must be a valid IPv4 or IPv6 address")
	}
	if controlAddr != "" {
		_ = saveNodeAddr(controlAddr)
	}

	if localUDPIP == "" {
		localUDPIP = detectOutboundLANIP()
	}

	log.Printf("[Bedrock Node Tunnel] Universal Open-Source Client %s (Raw UDP Fast-Lane) Starting...", version)
	log.Printf("[Bedrock Node Tunnel] Local TCP target: %s | Local UDP target IP: %s", localSignaling, localUDPIP)
	runClient(controlAddr, pollWebOverride, token, localSignaling, localUDPIP, localUDPDefault)
}

func validateLocalTCPTarget(raw string) error {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil || host == "" {
		return errors.New("target must be in host:port format")
	}
	if strings.ContainsAny(host, " \t\r\n/?#@") {
		return errors.New("target host contains invalid characters")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("target port must be between 1 and 65535")
	}
	return nil
}

func resolveNodeInput(input string) RegionNode {
	clean := strings.TrimSpace(input)
	upper := strings.ToUpper(clean)

	// Support friendly regional aliases
	switch upper {
	case "DALLAS", "TEXAS", "US-CENTRAL", "US-SOUTH":
		upper = "TX"
	case "LONDON", "EU", "EUROPE", "GB":
		upper = "UK"
	case "BRAZIL", "SA", "SOUTHA":
		upper = "BR"
	case "SG", "SINGAPORE", "AP":
		upper = "ASIA"
	case "USEAST", "US-EAST":
		upper = "US"
	}

	for _, n := range defaultRegionNodes {
		hostOnly := strings.TrimPrefix(strings.TrimPrefix(n.WebURL, "https://"), "http://")
		ipOnly := strings.TrimSuffix(n.ControlAddr, ":7000")
		if upper == n.Code || strings.EqualFold(clean, n.WebURL) || strings.EqualFold(clean, n.ControlAddr) ||
			strings.EqualFold(clean, hostOnly) || strings.EqualFold(clean, ipOnly) {
			return n
		}
	}

	clean = strings.TrimPrefix(clean, "https://")
	clean = strings.TrimPrefix(clean, "http://")
	clean = strings.TrimRight(clean, "/")

	host := clean
	ctrl := clean
	if strings.Contains(clean, ":") {
		h, p, err := net.SplitHostPort(clean)
		if err == nil {
			host = h
			if p == "8080" || p == "443" || p == "80" {
				ctrl = net.JoinHostPort(h, "7000")
			} else {
				ctrl = clean
			}
		}
	} else {
		ctrl = net.JoinHostPort(clean, "7000")
	}

	scheme := "https://"
	if net.ParseIP(host) != nil {
		scheme = "http://"
		return RegionNode{
			Code:        "CUSTOM",
			Name:        "Custom Node (" + host + ")",
			WebURL:      scheme + net.JoinHostPort(host, "8080"),
			ControlAddr: ctrl,
		}
	}

	return RegionNode{
		Code:        "CUSTOM",
		Name:        "Custom Node (" + host + ")",
		WebURL:      scheme + host,
		ControlAddr: ctrl,
	}
}

func getActiveNodeList(customControl, customWeb string) []RegionNode {
	if customControl != "" || customWeb != "" {
		control := customControl
		web := customWeb
		if control == "" && web != "" {
			control = resolveNodeInput(web).ControlAddr
		}
		if web == "" && control != "" {
			web = resolveNodeInput(control).WebURL
		}
		validControl, controlErr := validateControlAddr(control)
		validWeb, webErr := validateWebURL(web)
		if controlErr != nil || webErr != nil {
			return nil
		}
		return []RegionNode{{
			Code: "CUSTOM", Name: "Selected Node (" + validControl + ")",
			WebURL: validWeb, ControlAddr: validControl,
		}}
	}

	nodes := make([]RegionNode, 0, len(defaultRegionNodes))
	seenControl := make(map[string]bool)
	addNode := func(node RegionNode) {
		if node.ControlAddr == "" || seenControl[node.ControlAddr] {
			return
		}
		seenControl[node.ControlAddr] = true
		nodes = append(nodes, node)
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects while loading node metadata")
			}
			host := strings.ToLower(req.URL.Hostname())
			if req.URL.Scheme != "https" || !isBedrockNodeDomain(host) {
				return errors.New("node metadata redirect left the trusted HTTPS service domain")
			}
			return nil
		},
	}
	if resp, err := client.Get("https://bedrocknode.net/api/nodes"); err == nil {
		if resp.StatusCode == http.StatusOK {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
			if readErr == nil && len(data) <= 1<<20 {
				var remoteNodes []RegionNode
				if json.Unmarshal(data, &remoteNodes) == nil {
					for _, remote := range remoteNodes {
						if trusted, ok := sanitizeDiscoveredNode(remote); ok {
							addNode(trusted)
						}
					}
				}
			}
		}
		_ = resp.Body.Close()
	}

	// Built-in endpoints remain the trusted fallback if metadata is unavailable
	// or contains a region/address that fails validation.
	for _, node := range defaultRegionNodes {
		addNode(node)
	}
	return nodes
}

func isBedrockNodeDomain(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "bedrocknode.net" || strings.HasSuffix(host, ".bedrocknode.net")
}

func sanitizeDiscoveredNode(remote RegionNode) (RegionNode, bool) {
	code := strings.ToUpper(strings.TrimSpace(remote.Code))
	var fallback *RegionNode
	for i := range defaultRegionNodes {
		if defaultRegionNodes[i].Code == code {
			fallback = &defaultRegionNodes[i]
			break
		}
	}
	if fallback == nil {
		return RegionNode{}, false
	}

	webURL := fallback.WebURL
	if strings.TrimSpace(remote.WebURL) != "" {
		validated, err := validateWebURL(remote.WebURL)
		if err != nil {
			return RegionNode{}, false
		}
		parsed, err := url.Parse(validated)
		if err != nil || parsed.Scheme != "https" || !isBedrockNodeDomain(parsed.Hostname()) {
			return RegionNode{}, false
		}
		webURL = validated
	}

	control := fallback.ControlAddr
	if strings.TrimSpace(remote.ControlAddr) != "" {
		validated, err := validateControlAddr(remote.ControlAddr)
		if err != nil {
			return RegionNode{}, false
		}
		host, port, _ := net.SplitHostPort(validated)
		if port != "7000" {
			return RegionNode{}, false
		}
		if ip := net.ParseIP(host); ip != nil {
			// Dynamic IP changes require a client update; do not send tokens to
			// an arbitrary public IP returned by an HTTP metadata document.
			if validated != fallback.ControlAddr {
				return RegionNode{}, false
			}
		} else if !isBedrockNodeDomain(host) {
			return RegionNode{}, false
		}
		control = validated
	}

	name := strings.TrimSpace(remote.Name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || len(name) > 96 {
		name = fallback.Name
	}
	return RegionNode{Code: fallback.Code, Name: name, WebURL: webURL, ControlAddr: control}, true
}

func validateControlAddr(raw string) (string, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil || host == "" {
		return "", errors.New("control address must be in host:port format")
	}
	if strings.ContainsAny(host, " \t\r\n/?#@") {
		return "", errors.New("control host contains invalid characters")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("control port must be between 1 and 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func validateWebURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("URL scheme must be https (HTTP is allowed only for loopback testing)")
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", errors.New("URL host cannot be empty")
	}
	if u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("URL must not contain user information, fragments, or opaque data")
	}
	if u.RawQuery != "" {
		return "", errors.New("base dashboard URL must not contain a query string")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return "", errors.New("unencrypted HTTP is permitted only for loopback dashboard testing")
		}
	}
	if port := u.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", errors.New("URL port must be between 1 and 65535")
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func candidateTokenPaths() []string {
	var paths []string
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		paths = append(paths, filepath.Join(configDir, "bedrocknode-tunnel", tokenSaveFile))
	} else if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		paths = append(paths, filepath.Join(homeDir, ".config", "bedrocknode-tunnel", tokenSaveFile))
	}
	return append(paths, tokenSaveFile)
}

func candidateNodePaths() []string {
	var paths []string
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		paths = append(paths, filepath.Join(configDir, "bedrocknode-tunnel", nodeSaveFile))
	} else if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		paths = append(paths, filepath.Join(homeDir, ".config", "bedrocknode-tunnel", nodeSaveFile))
	}
	return append(paths, nodeSaveFile)
}

func loadSavedToken() (string, string) {
	for _, p := range candidateTokenPaths() {
		file, err := os.Open(p)
		if err != nil {
			continue
		}
		saved, ok := readToken(file)
		_ = file.Close()
		if ok {
			return saved, p
		}
	}
	return "", ""
}

func saveToken(tok string) string {
	if !isValidToken(tok) {
		return ""
	}
	for _, p := range candidateTokenPaths() {
		dir := filepath.Dir(p)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				continue
			}
		}
		if err := os.WriteFile(p, []byte(tok), 0600); err == nil {
			if err := os.Chmod(p, 0600); err == nil {
				return p
			}
			_ = os.Remove(p)
		}
	}
	return ""
}

func loadSavedNodeAddr() string {
	for _, p := range candidateNodePaths() {
		if data, err := os.ReadFile(p); err == nil && len(data) <= 4096 {
			raw := strings.TrimSpace(string(data))
			if raw != "" {
				control, err := validateControlAddr(resolveNodeInput(raw).ControlAddr)
				if err == nil {
					return control
				}
			}
		}
	}
	return ""
}

func saveNodeAddr(ctrlAddr string) string {
	ctrlAddr = strings.TrimSpace(ctrlAddr)
	if ctrlAddr == "" {
		return ""
	}
	for _, p := range candidateNodePaths() {
		dir := filepath.Dir(p)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				continue
			}
		}
		if err := os.WriteFile(p, []byte(ctrlAddr), 0600); err == nil {
			if err := os.Chmod(p, 0600); err == nil {
				return p
			}
			_ = os.Remove(p)
		}
	}
	return ""
}

func clearSavedTokens() {
	for _, p := range candidateTokenPaths() {
		_ = os.Remove(p)
	}
	for _, p := range candidateNodePaths() {
		_ = os.Remove(p)
	}
}

func splitOptionalNodeToken(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "|") {
		parts := strings.SplitN(s, "|", 2)
		nodePart := strings.TrimSpace(parts[0])
		tokenPart := strings.TrimSpace(parts[1])
		if nodePart != "" && isValidToken(tokenPart) {
			node := resolveNodeInput(nodePart)
			// Only built-in region aliases may override the control endpoint
			// from a token response. Custom endpoints must be explicitly set
			// by the user with -control.
			if node.Code != "CUSTOM" {
				return node.ControlAddr, tokenPart
			}
		}
	}
	return "", s
}

func openBrowser(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return
		}
		path, err := exec.LookPath("xdg-open")
		if err != nil {
			return
		}
		cmd = exec.Command(path, targetURL)
	default:
		return
	}
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}

func detectOutboundLANIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && !udpAddr.IP.IsLoopback() {
		return udpAddr.IP.String()
	}
	return "127.0.0.1"
}

func tuneTCPConn(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
		_ = tcpConn.SetReadBuffer(tcpSocketBufferBytes)
		_ = tcpConn.SetWriteBuffer(tcpSocketBufferBytes)
	}
}

func tuneUDPConn(conn *net.UDPConn, bufBytes int) {
	if conn != nil {
		_ = conn.SetReadBuffer(bufBytes)
		_ = conn.SetWriteBuffer(bufBytes)
	}
}

func newYamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 35 * time.Second
	cfg.MaxStreamWindowSize = 4 * 1024 * 1024
	return cfg
}

func buildLocalUDPCandidates(localUDPIP string, targetPort int) []candConn {
	type targetSpec struct {
		netw string
		addr string
	}
	specs := []targetSpec{
		{"udp4", net.JoinHostPort(localUDPIP, fmt.Sprintf("%d", targetPort))},
		{"udp4", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", targetPort))},
		{"udp6", net.JoinHostPort("::1", fmt.Sprintf("%d", targetPort))},
	}

	var conns []candConn
	seen := make(map[string]bool)
	for _, s := range specs {
		if seen[s.addr] {
			continue
		}
		seen[s.addr] = true
		rAddr, err := net.ResolveUDPAddr(s.netw, s.addr)
		if err != nil {
			continue
		}
		c, err := net.DialUDP(s.netw, nil, rAddr)
		if err != nil {
			continue
		}
		tuneUDPConn(c, udpSocketBufferBytes)
		conns = append(conns, candConn{label: s.addr, conn: c})
	}
	return conns
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func isValidToken(tok string) bool {
	if len(tok) == 0 || len(tok) > maxTokenLength {
		return false
	}
	for _, r := range tok {
		if r <= 0x20 || r >= 0x7f {
			return false
		}
	}
	return true
}

func readToken(r io.Reader) (string, bool) {
	body, err := io.ReadAll(io.LimitReader(r, maxTokenLength+1))
	if err != nil || len(body) > maxTokenLength {
		return "", false
	}
	tok := strings.TrimSpace(string(body))
	return tok, isValidToken(tok)
}

func obtainOrClaimToken(customControl, customWeb, flagToken string) (string, string) {
	if isValidToken(flagToken) {
		embCtrl, cleanTok := splitOptionalNodeToken(flagToken)
		if embCtrl == "" {
			embCtrl = customControl
		}
		if embCtrl == "" {
			embCtrl = loadSavedNodeAddr()
		}
		return embCtrl, cleanTok
	}

	if saved, path := loadSavedToken(); saved != "" {
		embCtrl, cleanTok := splitOptionalNodeToken(saved)
		if embCtrl == "" {
			embCtrl = customControl
		}
		if embCtrl == "" {
			embCtrl = loadSavedNodeAddr()
		}
		log.Printf("[Bedrock Node Tunnel] Loaded saved tunnel token from %s", path)
		return embCtrl, cleanTok
	}

	nodes := getActiveNodeList(customControl, customWeb)
	if len(nodes) == 0 {
		log.Fatalf("[Bedrock Node Tunnel] No trusted HTTPS claim dashboard is configured. For custom nodes, pass both -control host:port and -web https://your-dashboard.")
		return "", ""
	}
	claimCode := strings.ToUpper(randomHex(3))
	if claimCode == "" {
		log.Fatal("[Bedrock Node Tunnel] Could not generate a secure claim code.")
		return "", ""
	}

	fmt.Println("==========================================================================")
	fmt.Println("  BEDROCK NODE TUNNEL — FIRST TIME SETUP (MULTI-REGION)")
	fmt.Println("==========================================================================")
	fmt.Printf("  Claim Code: %s\n\n", claimCode)
	fmt.Println("  Open your closest regional dashboard to claim this tunnel:")
	for _, n := range nodes {
		if n.WebURL != "" {
			fmt.Printf("  * %-32s %s/?claim=%s\n", n.Name+":", strings.TrimRight(n.WebURL, "/"), url.QueryEscape(claimCode))
		}
	}
	fmt.Println("==========================================================================")

	if len(nodes) > 0 && nodes[0].WebURL != "" {
		firstLink := fmt.Sprintf("%s/?claim=%s", strings.TrimRight(nodes[0].WebURL, "/"), url.QueryEscape(claimCode))
		openBrowser(firstLink)
	}

	for {
		for _, n := range nodes {
			if n.WebURL == "" {
				continue
			}
			baseURL, parseErr := url.Parse(n.WebURL)
			if parseErr != nil {
				continue
			}
			client := &http.Client{
				Timeout: 4 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					if len(via) >= 3 || req.URL.Scheme != baseURL.Scheme ||
						!strings.EqualFold(req.URL.Host, baseURL.Host) {
						return errors.New("claim poll redirected outside the configured dashboard origin")
					}
					return nil
				},
			}
			pollUrl := fmt.Sprintf("%s/api/poll-claim?code=%s", strings.TrimRight(n.WebURL, "/"), url.QueryEscape(claimCode))
			resp, err := client.Get(pollUrl)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					tok, ok := readToken(resp.Body)
					resp.Body.Close()
					if ok {
						embCtrl, cleanTok := splitOptionalNodeToken(tok)
						chosenCtrl := embCtrl
						if chosenCtrl == "" {
							chosenCtrl = n.ControlAddr
						}
						if savedPath := saveToken(cleanTok); savedPath != "" {
							log.Printf("[Bedrock Node Tunnel] Tunnel claimed on %s! Saved token to %s.", n.Name, savedPath)
						}
						_ = saveNodeAddr(chosenCtrl)
						return chosenCtrl, cleanTok
					}
				} else {
					resp.Body.Close()
				}
			}
		}
		time.Sleep(2500 * time.Millisecond)
	}
}

func sendTunnelToken(conn net.Conn, token string) error {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	if err := writeFramedPacket(conn, []byte(token)); err != nil {
		return err
	}
	var ack [1]byte
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return err
	}
	if ack[0] != 0x01 {
		return errInvalidToken
	}
	return nil
}

func writeFramedPacket(w io.Writer, payload []byte) error {
	if len(payload) > 65535 {
		return fmt.Errorf("packet too large: %d", len(payload))
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if err := writeAll(w, hdr[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func readFramedPacket(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(hdr[:])
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func runClient(customControl, customWeb, flagToken, localSignaling, localUDPIP string, localUDPDefault int) {
	targetControl, token := obtainOrClaimToken(customControl, customWeb, flagToken)
	handshakeFailures := 0

	for {
		var authConn net.Conn
		var activeControl string

		// 1. If we already know the regional control address, dial it directly
		if targetControl != "" {
			log.Printf("[Tunnel Client] Dialing Bedrock Node Tunnel VPS at %s...", targetControl)
			conn, err := net.DialTimeout("tcp", targetControl, 10*time.Second)
			if err != nil {
				log.Printf("[Tunnel Client] Connection to %s failed (%v). Retrying in 3s...", targetControl, err)
				time.Sleep(3 * time.Second)
				continue
			}
			tuneTCPConn(conn)

			if err := sendTunnelToken(conn, token); err != nil {
				conn.Close()
				if errors.Is(err, errInvalidToken) {
					if customControl != "" {
						log.Printf("[Tunnel Client] Auth error on %s (%v) - deleting saved token and re-claiming...", targetControl, err)
						clearSavedTokens()
						targetControl, token = obtainOrClaimToken(customControl, customWeb, "")
						handshakeFailures = 0
						time.Sleep(3 * time.Second)
						continue
					}
					log.Printf("[Tunnel Client] Token not found on %s — checking other regional hubs...", targetControl)
					targetControl = ""
				} else {
					handshakeFailures++
					log.Printf("[Tunnel Client] Handshake failed (%v). Retrying in 3s...", err)
					if handshakeFailures%handshakeHint == 0 {
						log.Printf("[Tunnel Client] The server closed the connection %d times in a row during login. If your token may be invalid, restart with -reset-token to claim a new one.", handshakeFailures)
					}
					time.Sleep(3 * time.Second)
					continue
				}
			} else {
				authConn = conn
				activeControl = targetControl
				handshakeFailures = 0
			}
		}

		// 2. If region is unknown (e.g., user placed bedrocknode-token.txt in folder), probe all regional hubs
		if authConn == nil {
			nodes := getActiveNodeList(customControl, customWeb)
			reachable := 0
			rejected := 0

			for _, n := range nodes {
				log.Printf("[Tunnel Client] Checking regional hub %s (%s)...", n.Name, n.ControlAddr)
				conn, err := net.DialTimeout("tcp", n.ControlAddr, 6*time.Second)
				if err != nil {
					continue
				}
				tuneTCPConn(conn)
				reachable++

				err = sendTunnelToken(conn, token)
				if err == nil {
					authConn = conn
					activeControl = n.ControlAddr
					targetControl = n.ControlAddr
					_ = saveNodeAddr(targetControl)
					handshakeFailures = 0
					log.Printf("[Tunnel Client] Authenticated token on %s (%s)!", n.Name, n.ControlAddr)
					break
				}
				conn.Close()
				if errors.Is(err, errInvalidToken) {
					rejected++
				}
			}

			if authConn == nil {
				if reachable > 0 && rejected == reachable {
					log.Printf("[Tunnel Client] Token rejected by all %d reachable regional hubs - deleting saved token and re-claiming...", reachable)
					clearSavedTokens()
					targetControl, token = obtainOrClaimToken(customControl, customWeb, "")
					handshakeFailures = 0
					time.Sleep(3 * time.Second)
					continue
				}
				handshakeFailures++
				log.Printf("[Tunnel Client] Could not complete handshake with regional hubs yet. Retrying in 3s...")
				time.Sleep(3 * time.Second)
				continue
			}
		}

		sess, err := yamux.Client(authConn, newYamuxConfig())
		if err != nil {
			log.Printf("[Tunnel Client] Yamux client error: %v", err)
			authConn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[Tunnel Client] Authenticated tunnel established via %s! TCP -> %s | UDP -> %s", activeControl, localSignaling, localUDPIP)

		stopRawUDP := make(chan struct{})
		go runRawUDPFastLane(stopRawUDP, activeControl, token, localUDPIP, localUDPDefault)

		for {
			stream, err := sess.Accept()
			if err != nil {
				log.Printf("[Tunnel Client] Tunnel disconnected (%v). Reconnecting...", err)
				break
			}
			go handleIncomingStream(stream, localSignaling, localUDPIP, localUDPDefault)
		}

		close(stopRawUDP)
		_ = sess.Close()
	}
}

func runRawUDPFastLane(stopCh <-chan struct{}, controlAddr, token, localUDPIP string, localUDPDefault int) {
	rAddr, err := net.ResolveUDPAddr("udp4", controlAddr)
	if err != nil {
		return
	}
	rawConn, err := net.DialUDP("udp4", nil, rAddr)
	if err != nil {
		return
	}
	tuneUDPConn(rawConn, rawUDPSocketBufferBytes)

	var (
		sessionsMu sync.Mutex
		sessions   = make(map[rawPlayerKey]*rawLocalSession)
		activeFlag atomic.Bool
	)

	defer func() {
		_ = rawConn.Close()
		sessionsMu.Lock()
		for k, s := range sessions {
			s.Close()
			delete(sessions, k)
		}
		sessionsMu.Unlock()
	}()

	go func() {
		<-stopCh
		_ = rawConn.Close()
	}()

	// Send immediate registration heartbeat + repeat every 8 seconds to keep NAT hole & server fast-lane open
	hbPkt := append([]byte{RawUDPMagicHeartbeat}, []byte(token)...)
	_, _ = rawConn.Write(hbPkt)

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				_, _ = rawConn.Write(hbPkt)
			}
		}
	}()

	// Clean up idle player UDP sessions every 30s
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				now := time.Now().UnixNano()
				sessionsMu.Lock()
				for k, s := range sessions {
					if now-s.lastActive.Load() > int64(2*time.Minute) {
						s.Close()
						delete(sessions, k)
					}
				}
				sessionsMu.Unlock()
			}
		}
	}()

	buf := make([]byte, 65535)
	for {
		n, err := rawConn.Read(buf)
		if err != nil {
			select {
			case <-stopCh:
				return
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		if n < 2 {
			continue
		}

		switch buf[0] {
		case RawUDPMagicHeartbeat:
			if n >= 2 && buf[1] == 0x01 && !activeFlag.Swap(true) {
				log.Printf("[Tunnel Client] 🚀 RAW UDP FAST-LANE ACTIVE on %s (UDP traffic bypassed yamux!)", controlAddr)
			}

		case RawUDPMagicData:
			if n <= 9 {
				continue
			}
			pubPort := binary.BigEndian.Uint16(buf[1:3])
			var key rawPlayerKey
			key.pubPort = pubPort
			copy(key.playerIP[:], buf[3:7])
			key.playerPort = binary.BigEndian.Uint16(buf[7:9])
			payload := buf[9:n]

			targetPort := int(pubPort)
			if targetPort == 0 {
				targetPort = localUDPDefault
			}

			nowNano := time.Now().UnixNano()
			sessionsMu.Lock()
			sess, exists := sessions[key]
			if !exists {
				// Bound the number of player sessions and local UDP sockets this
				// process can create from incoming relay traffic.
				if len(sessions) >= maxRawPlayerSessions {
					var oldestKey rawPlayerKey
					var oldestSession *rawLocalSession
					oldestTime := nowNano
					for key, candidate := range sessions {
						lastActive := candidate.lastActive.Load()
						if oldestSession == nil || lastActive < oldestTime {
							oldestKey, oldestSession, oldestTime = key, candidate, lastActive
						}
					}
					if oldestSession != nil {
						oldestSession.Close()
						delete(sessions, oldestKey)
					}
				}
				conns := buildLocalUDPCandidates(localUDPIP, targetPort)
				if len(conns) == 0 {
					sessionsMu.Unlock()
					continue
				}
				sess = &rawLocalSession{conns: conns}
				sess.winnerIdx.Store(-1)
				sess.lastActive.Store(nowNano)
				sessions[key] = sess

				for idx, cc := range conns {
					go func(i int, c candConn, s *rawLocalSession, k rawPlayerKey) {
						replyBuf := make([]byte, 65535)
						outFrame := make([]byte, maxUDPPacketPayload)
						outFrame[0] = RawUDPMagicData
						binary.BigEndian.PutUint16(outFrame[1:3], k.pubPort)
						copy(outFrame[3:7], k.playerIP[:])
						binary.BigEndian.PutUint16(outFrame[7:9], k.playerPort)

						for {
							_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
							rn, rErr := c.conn.Read(replyBuf)
							if rErr != nil {
								errStr := rErr.Error()
								if strings.Contains(errStr, "forcibly closed") || strings.Contains(errStr, "connection refused") {
									continue
								}
								return
							}
							s.lastActive.Store(time.Now().UnixNano())
							if s.winnerIdx.Swap(int32(i)) == -1 {
								log.Printf("[Tunnel Client] <- [RAW UDP] WebRTC reply (%d bytes) from Bedrock at %s", rn, c.label)
							}
							if rn > maxUDPPacketPayload-9 {
								continue
							}
							copy(outFrame[9:9+rn], replyBuf[:rn])
							_, _ = rawConn.Write(outFrame[:9+rn])
						}
					}(idx, cc, sess, key)
				}
			}
			sess.lastActive.Store(nowNano)
			sessionsMu.Unlock()

			wIdx := int(sess.winnerIdx.Load())
			if wIdx >= 0 && wIdx < len(sess.conns) {
				_, _ = sess.conns[wIdx].conn.Write(payload)
			} else {
				for _, c := range sess.conns {
					_, _ = c.conn.Write(payload)
				}
			}
		}
	}
}

func handleIncomingStream(stream net.Conn, localSignaling, localUDPIP string, localUDPDefault int) {
	defer stream.Close()

	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, typeBuf); err != nil {
		return
	}

	switch typeBuf[0] {
	case StreamTypeHTTP, StreamTypeTCP, StreamTypeTCPMapped:
		targetSignaling := localSignaling
		if typeBuf[0] == StreamTypeTCPMapped {
			var portBuf [2]byte
			if _, err := io.ReadFull(stream, portBuf[:]); err != nil {
				return
			}
			mappedPort := int(binary.BigEndian.Uint16(portBuf[:]))
			if mappedPort > 0 {
				host, _, err := net.SplitHostPort(localSignaling)
				if err != nil || host == "" {
					host = "127.0.0.1"
				}
				targetSignaling = net.JoinHostPort(host, fmt.Sprintf("%d", mappedPort))
			}
		}

		localConn, err := net.DialTimeout("tcp", targetSignaling, 5*time.Second)
		if err != nil {
			log.Printf("[Tunnel Client] Local Bedrock TCP server unreachable at %s: %v", targetSignaling, err)
			return
		}
		defer localConn.Close()
		tuneTCPConn(localConn)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			buf := make([]byte, 65536)
			_, _ = io.CopyBuffer(localConn, stream, buf)
			if tc, ok := localConn.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
		}()
		go func() {
			defer wg.Done()
			buf := make([]byte, 65536)
			_, _ = io.CopyBuffer(stream, localConn, buf)
			_ = stream.Close()
		}()
		wg.Wait()

	case StreamTypeUDP:
		// Automatic Yamux fallback if UDP port 7000 is blocked by a strict local firewall
		var portBuf [2]byte
		if _, err := io.ReadFull(stream, portBuf[:]); err != nil {
			return
		}
		targetPort := int(binary.BigEndian.Uint16(portBuf[:]))
		if targetPort == 0 {
			targetPort = localUDPDefault
		}

		conns := buildLocalUDPCandidates(localUDPIP, targetPort)
		if len(conns) == 0 {
			return
		}
		defer func() {
			for _, c := range conns {
				_ = c.conn.Close()
			}
		}()

		var (
			writeMu   sync.Mutex
			winnerIdx atomic.Int32
		)
		winnerIdx.Store(-1)

		for idx, cc := range conns {
			go func(i int, c candConn) {
				buf := make([]byte, 65535)
				for {
					_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
					n, err := c.conn.Read(buf)
					if err != nil {
						errStr := err.Error()
						if strings.Contains(errStr, "forcibly closed") || strings.Contains(errStr, "connection refused") {
							time.Sleep(50 * time.Millisecond)
							continue
						}
						return
					}
					if winnerIdx.Swap(int32(i)) == -1 {
						log.Printf("[Tunnel Client] <- [Yamux Fallback] Received first WebRTC UDP reply (%d bytes) from Bedrock at %s!", n, c.label)
					}
					writeMu.Lock()
					wErr := writeFramedPacket(stream, buf[:n])
					writeMu.Unlock()
					if wErr != nil {
						_ = stream.Close()
						return
					}
				}
			}(idx, cc)
		}

		for {
			pkt, err := readFramedPacket(stream)
			if err != nil {
				break
			}
			wIdx := int(winnerIdx.Load())
			if wIdx >= 0 && wIdx < len(conns) {
				_ = conns[wIdx].conn.SetWriteDeadline(time.Now().Add(2 * time.Minute))
				_, _ = conns[wIdx].conn.Write(pkt)
				continue
			}
			for _, c := range sessConns(conns) {
				_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Minute))
				_, _ = c.conn.Write(pkt)
			}
		}
	}
}

func sessConns(conns []candConn) []candConn {
	return conns
}
