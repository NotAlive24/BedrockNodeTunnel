package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
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
	maxTokenLength = 4096
	handshakeHint  = 5
)

var version = "dev"

var errInvalidToken = errors.New("invalid auth token rejected by server")

func main() {
	var (
		controlAddr     string
		webUrl          string
		token           string
		localSignaling  string
		localUDPIP      string
		localUDPDefault int
		resetToken      bool
		showVersion     bool
	)

	flag.StringVar(&controlAddr, "control", "51.222.142.179:7000", "Bedrock Node Tunnel VPS control address")
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

	if localUDPIP == "" {
		localUDPIP = detectOutboundLANIP()
	}

	log.Printf("[Bedrock Node Tunnel] Open-Source Client %s Starting...", version)
	log.Printf("[Bedrock Node Tunnel] Local TCP target: %s | Local UDP target IP: %s", localSignaling, localUDPIP)
	runClient(controlAddr, cleanWebURL, token, localSignaling, localUDPIP, localUDPDefault)
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

func clearSavedTokens() {
	for _, p := range candidateTokenPaths() {
		_ = os.Remove(p)
	}
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

func obtainOrClaimToken(webUrl, flagToken string) string {
	if isValidToken(flagToken) {
		return flagToken
	}
	if saved, path := loadSavedToken(); saved != "" {
		log.Printf("[Bedrock Node Tunnel] Loaded saved tunnel token from %s", path)
		return saved
	}

	claimCode := strings.ToUpper(randomHex(3))
	claimLink := fmt.Sprintf("%s/?claim=%s", strings.TrimRight(webUrl, "/"), url.QueryEscape(claimCode))

	fmt.Println("==========================================================================")
	fmt.Println("  BEDROCK NODE TUNNEL — FIRST TIME SETUP")
	fmt.Println("==========================================================================")
	fmt.Printf("  1. Open this link in your browser to claim your tunnel:\n")
	fmt.Printf("     %s\n\n", claimLink)
	fmt.Printf("  2. Or log into %s and enter Claim Code: %s\n", webUrl, claimCode)
	fmt.Println("==========================================================================")

	openBrowser(claimLink)

	pollUrl := fmt.Sprintf("%s/api/poll-claim?code=%s", strings.TrimRight(webUrl, "/"), url.QueryEscape(claimCode))
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		resp, err := client.Get(pollUrl)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				tok, ok := readToken(resp.Body)
				resp.Body.Close()
				if ok {
					if savedPath := saveToken(tok); savedPath != "" {
						log.Printf("[Bedrock Node Tunnel] Tunnel claimed! Saved token to %s.", savedPath)
					}
					return tok
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(3 * time.Second)
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

func runClient(controlAddr, webUrl, flagToken, localSignaling, localUDPIP string, localUDPDefault int) {
	token := obtainOrClaimToken(webUrl, flagToken)
	handshakeFailures := 0

	for {
		log.Printf("[Tunnel Client] Dialing Bedrock Node Tunnel VPS at %s...", controlAddr)
		conn, err := net.DialTimeout("tcp", controlAddr, 10*time.Second)
		if err != nil {
			log.Printf("[Tunnel Client] Connection failed (%v). Retrying in 3s...", err)
			time.Sleep(3 * time.Second)
			continue
		}

		if err := sendTunnelToken(conn, token); err != nil {
			conn.Close()
			if errors.Is(err, errInvalidToken) {
				log.Printf("[Tunnel Client] Auth error (%v) - deleting saved token and re-claiming...", err)
				clearSavedTokens()
				token = obtainOrClaimToken(webUrl, "")
				handshakeFailures = 0
				time.Sleep(3 * time.Second)
			} else {
				handshakeFailures++
				log.Printf("[Tunnel Client] Handshake failed (%v). Retrying in 3s...", err)
				if handshakeFailures%handshakeHint == 0 {
					log.Printf("[Tunnel Client] The server closed the connection %d times in a row during login. If your token may be invalid, restart with -reset-token to claim a new one.", handshakeFailures)
				}
				time.Sleep(3 * time.Second)
			}
			continue
		}
		handshakeFailures = 0

		sess, err := yamux.Client(conn, newYamuxConfig())
		if err != nil {
			log.Printf("[Tunnel Client] Yamux client error: %v", err)
			conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[Tunnel Client] Authenticated tunnel established! TCP -> %s | UDP -> %s", localSignaling, localUDPIP)

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
