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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
)

const (
	StreamTypeHTTP = 0x01
	StreamTypeUDP  = 0x02
	StreamTypeTCP  = 0x03
	tokenSaveFile  = "bedrocknode-token.txt"
	nodeSaveFile   = "bedrocknode-node.txt"
	maxTokenLength = 4096
	handshakeHint  = 5
)

var version = "dev"

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
		Name:        "North America (US Hub)",
		WebURL:      "https://bedrocknode.net",
		ControlAddr: "51.222.142.179:7000",
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

	flag.StringVar(&nodeFlag, "node", "", "Region code (US, BR, ASIA) or custom VPS domain/IP (e.g. asia.bedrocknode.net)")
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
		_ = saveNodeAddr(controlAddr)
	} else if explicitFlags["control"] && !explicitFlags["web"] {
		n := resolveNodeInput(controlAddr)
		controlAddr = n.ControlAddr
		if n.WebURL != "" {
			cleanWebURL = n.WebURL
		}
		pollWebOverride = cleanWebURL
		_ = saveNodeAddr(controlAddr)
	} else if explicitFlags["web"] && !explicitFlags["control"] {
		n := resolveNodeInput(cleanWebURL)
		controlAddr = n.ControlAddr
		pollWebOverride = cleanWebURL
		_ = saveNodeAddr(controlAddr)
	} else if explicitFlags["web"] && explicitFlags["control"] {
		pollWebOverride = cleanWebURL
		_ = saveNodeAddr(controlAddr)
	}

	if localUDPIP == "" {
		localUDPIP = detectOutboundLANIP()
	}

	log.Printf("[Bedrock Node Tunnel] Universal Open-Source Client %s Starting...", version)
	log.Printf("[Bedrock Node Tunnel] Local TCP target: %s | Local UDP target IP: %s", localSignaling, localUDPIP)
	runClient(controlAddr, pollWebOverride, token, localSignaling, localUDPIP, localUDPDefault)
}

func resolveNodeInput(input string) RegionNode {
	clean := strings.TrimSpace(input)
	upper := strings.ToUpper(clean)

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
	// If the user explicitly passed a region via -node, -control, or -web, use that specific node
	if customControl != "" || customWeb != "" {
		cAddr := customControl
		wURL := customWeb
		if cAddr == "" && wURL != "" {
			cAddr = resolveNodeInput(wURL).ControlAddr
		}
		if wURL == "" && cAddr != "" {
			wURL = resolveNodeInput(cAddr).WebURL
		}
		return []RegionNode{
			{
				Code:        "CUSTOM",
				Name:        "Selected Node (" + cAddr + ")",
				WebURL:      strings.TrimRight(wURL, "/"),
				ControlAddr: cAddr,
			},
		}
	}

	var nodes []RegionNode
	seenCtrl := make(map[string]bool)
	addNode := func(n RegionNode) {
		if n.ControlAddr == "" || seenCtrl[n.ControlAddr] {
			return
		}
		seenCtrl[n.ControlAddr] = true
		nodes = append(nodes, n)
	}

	// Optional live discovery for future nodes if /api/nodes is added later
	client := &http.Client{Timeout: 2 * time.Second}
	if resp, err := client.Get("https://bedrocknode.net/api/nodes"); err == nil {
		if resp.StatusCode == http.StatusOK {
			var remoteNodes []RegionNode
			if json.NewDecoder(resp.Body).Decode(&remoteNodes) == nil {
				for _, rn := range remoteNodes {
					if rn.ControlAddr == "" && rn.WebURL != "" {
						rn = resolveNodeInput(rn.WebURL)
					}
					addNode(rn)
				}
			}
		}
		resp.Body.Close()
	}

	for _, n := range defaultRegionNodes {
		addNode(n)
	}
	return nodes
}

func validateWebURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("URL scheme must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("URL host cannot be empty")
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
		if data, err := os.ReadFile(p); err == nil {
			saved := strings.TrimSpace(string(data))
			if isValidToken(saved) {
				return saved, p
			}
		}
	}
	return "", ""
}

func saveToken(tok string) string {
	for _, p := range candidateTokenPaths() {
		dir := filepath.Dir(p)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				continue
			}
		}
		if err := os.WriteFile(p, []byte(tok), 0600); err == nil {
			return p
		}
	}
	return ""
}

func loadSavedNodeAddr() string {
	for _, p := range candidateNodePaths() {
		if data, err := os.ReadFile(p); err == nil {
			raw := strings.TrimSpace(string(data))
			if raw != "" {
				return resolveNodeInput(raw).ControlAddr
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
			return p
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
		nPart := strings.TrimSpace(parts[0])
		tPart := strings.TrimSpace(parts[1])
		if nPart != "" && isValidToken(tPart) {
			return resolveNodeInput(nPart).ControlAddr, tPart
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

func newYamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 15 * time.Second
	cfg.ConnectionWriteTimeout = 15 * time.Second
	return cfg
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
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
	claimCode := strings.ToUpper(randomHex(3))

	fmt.Println("==========================================================================")
	fmt.Println("  BEDROCK NODE TUNNEL â€” FIRST TIME SETUP (MULTI-REGION)")
	fmt.Println("==========================================================================")
	fmt.Printf("  Claim Code: %s\n\n", claimCode)
	fmt.Println("  Open your closest regional dashboard to claim this tunnel:")
	for _, n := range nodes {
		if n.WebURL != "" {
			fmt.Printf("  â€¢ %-30s %s/?claim=%s\n", n.Name+":", strings.TrimRight(n.WebURL, "/"), url.QueryEscape(claimCode))
		}
	}
	fmt.Println("==========================================================================")

	if len(nodes) > 0 && nodes[0].WebURL != "" {
		firstLink := fmt.Sprintf("%s/?claim=%s", strings.TrimRight(nodes[0].WebURL, "/"), url.QueryEscape(claimCode))
		openBrowser(firstLink)
	}

	client := &http.Client{Timeout: 4 * time.Second}

	for {
		for _, n := range nodes {
			if n.WebURL == "" {
				continue
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
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
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
					log.Printf("[Tunnel Client] Token not found on %s â€” checking other regional hubs...", targetControl)
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

		for {
			stream, err := sess.Accept()
			if err != nil {
				log.Printf("[Tunnel Client] Tunnel disconnected (%v). Reconnecting...", err)
				_ = sess.Close()
				break
			}
			go handleIncomingStream(stream, localSignaling, localUDPIP, localUDPDefault)
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
	case StreamTypeHTTP, StreamTypeTCP:
		log.Printf("[Tunnel Client] Forwarding TCP/Signaling stream -> %s", localSignaling)
		localConn, err := net.DialTimeout("tcp", localSignaling, 5*time.Second)
		if err != nil {
			log.Printf("[Tunnel Client] Local Bedrock TCP server unreachable at %s: %v", localSignaling, err)
			return
		}
		defer localConn.Close()

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(localConn, stream)
			if tc, ok := localConn.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
		}()
		go func() {
			defer wg.Done()
			_, _ = io.Copy(stream, localConn)
			_ = stream.Close()
		}()
		wg.Wait()

	case StreamTypeUDP:
		var portBuf [2]byte
		if _, err := io.ReadFull(stream, portBuf[:]); err != nil {
			return
		}
		targetPort := int(binary.BigEndian.Uint16(portBuf[:]))
		if targetPort == 0 {
			targetPort = localUDPDefault
		}

		type targetSpec struct {
			netw string
			addr string
		}
		specs := []targetSpec{
			{"udp4", net.JoinHostPort(localUDPIP, fmt.Sprintf("%d", targetPort))},
			{"udp4", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", targetPort))},
			{"udp6", net.JoinHostPort("::1", fmt.Sprintf("%d", targetPort))},
		}

		type candConn struct {
			label string
			conn  *net.UDPConn
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
			conns = append(conns, candConn{label: s.addr, conn: c})
		}
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
						log.Printf("[Tunnel Client] <- SUCCESS! Received first WebRTC UDP reply (%d bytes) from Bedrock at %s!", n, c.label)
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
			for _, c := range conns {
				_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Minute))
				_, _ = c.conn.Write(pkt)
			}
		}
	}
}
