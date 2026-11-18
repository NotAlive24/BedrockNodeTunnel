package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	desktopVersion = "1.0.0"
	upstreamRepo   = "NotAlive24/BedrockNodeTunnel"
	releaseAPI     = "https://api.github.com/repos/" + upstreamRepo + "/releases/latest"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var secretPattern = regexp.MustCompile("(?i)(token\\s*[:=]\\s*)[^\\s,;]+")

type Profile struct {
	ID           string \`json:"id"\`
	Name         string \`json:"name"\`
	Region       string \`json:"region"\`
	LocalTCP     string \`json:"localTCP"\`
	LocalUDPIP   string \`json:"localUDPIP"\`
	LocalUDPPort int    \`json:"localUDPPort"\`
}

type AppSettings struct {
	EnginePath string \`json:"enginePath"\`
}

type appConfig struct {
	SchemaVersion int         \`json:"schemaVersion"\`
	ActiveProfile string      \`json:"activeProfile"\`
	Settings      AppSettings \`json:"settings"\`
	Profiles      []Profile   \`json:"profiles"\`
}

type LogEntry struct {
	ID      int64  \`json:"id"\`
	Time    string \`json:"time"\`
	Level   string \`json:"level"\`
	Message string \`json:"message"\`
}

type DiagnosticResult struct {
	Name    string \`json:"name"\`
	Status  string \`json:"status"\`
	Details string \`json:"details"\`
}

type EngineInfo struct {
	Path    string \`json:"path"\`
	Version string \`json:"version"\`
	Updated bool   \`json:"updated"\`
}

type AppSnapshot struct {
	AppVersion          string   \`json:"appVersion"\`
	EngineFound         bool     \`json:"engineFound"\`
	EnginePath          string   \`json:"enginePath"\`
	EngineVersion       string   \`json:"engineVersion"\`
	Running             bool     \`json:"running"\`
	BackgroundRunning   bool     \`json:"backgroundRunning"\`
	BackgroundSupported bool     \`json:"backgroundSupported"\`
	Mode                string   \`json:"mode"\`
	ConnectionState     string   \`json:"connectionState"\`
	Uptime              string   \`json:"uptime"\`
	StartedAt           string   \`json:"startedAt"\`
	LastError           string   \`json:"lastError"\`
	ActiveProfile       Profile  \`json:"activeProfile"\`
	Profiles            []Profile \`json:"profiles"\`
}

type AppService struct {
	mu         sync.RWMutex
	configDir  string
	configPath string
	config     appConfig
	cmd        *exec.Cmd
	done       chan struct{}
	startedAt  time.Time
	logs       []LogEntry
	nextLogID  int64
	lastError  string
	connection string
}

func NewAppService() *AppService {
	root, err := os.UserConfigDir()
	if err != nil || root == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil { home = "." }
		root = filepath.Join(home, ".config")
	}
	dir := filepath.Join(root, "bedrocknode-tunnel")
	_ = os.MkdirAll(dir, 0700)
	s := &AppService{configDir: dir, configPath: filepath.Join(dir, "desktop.json"), connection: "Stopped"}
	s.loadConfig()
	return s
}

func (s *AppService) loadConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.configPath)
	if err == nil { _ = json.Unmarshal(data, &s.config) }
	if s.config.SchemaVersion == 0 { s.config.SchemaVersion = 1 }
	if len(s.config.Profiles) == 0 {
		p := defaultProfile()
		s.config.Profiles, s.config.ActiveProfile = []Profile{p}, p.ID
	}
	if s.findProfileLocked(s.config.ActiveProfile) == nil { s.config.ActiveProfile = s.config.Profiles[0].ID }
	_ = s.saveConfigLocked()
}

func defaultProfile() Profile {
	return Profile{ID:"default", Name:"Default", Region:"AUTO", LocalTCP:"127.0.0.1:19132", LocalUDPPort:47334}
}

func (s *AppService) saveConfigLocked() error {
	s.config.SchemaVersion = 1
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil { return err }
	tmp := s.configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil { return err }
	return os.Rename(tmp, s.configPath)
}

func (s *AppService) findProfileLocked(id string) *Profile {
	for i := range s.config.Profiles {
		if s.config.Profiles[i].ID == id { return &s.config.Profiles[i] }
	}
	return nil
}

func (s *AppService) activeProfileLocked() Profile {
	if p := s.findProfileLocked(s.config.ActiveProfile); p != nil { return *p }
	if len(s.config.Profiles) > 0 { return s.config.Profiles[0] }
	return defaultProfile()
}

func (s *AppService) GetProfiles() []Profile {
	s.mu.RLock(); defer s.mu.RUnlock()
	return append([]Profile(nil), s.config.Profiles...)
}

func (s *AppService) SaveProfile(profile Profile) ([]Profile, error) {
	if err := validateProfile(profile); err != nil { return nil, err }
	s.mu.Lock(); defer s.mu.Unlock()
	if profile.ID == "" { profile.ID = fmt.Sprintf("profile-%d", time.Now().UnixNano()) }
	found := false
	for i := range s.config.Profiles {
		if s.config.Profiles[i].ID == profile.ID {
			s.config.Profiles[i], found = profile, true
			break
		}
	}
	if !found { s.config.Profiles = append(s.config.Profiles, profile) }
	if s.config.ActiveProfile == "" { s.config.ActiveProfile = profile.ID }
	if err := s.saveConfigLocked(); err != nil { return nil, err }
	return append([]Profile(nil), s.config.Profiles...), nil
}

func (s *AppService) SetActiveProfile(id string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.findProfileLocked(id) == nil { return errors.New("profile not found") }
	if s.cmd != nil { return errors.New("stop the foreground tunnel before switching profiles") }
	if active, _ := backgroundStatus(); active { return errors.New("stop the background tunnel before switching profiles") }
	s.config.ActiveProfile = id
	return s.saveConfigLocked()
}

func (s *AppService) DeleteProfile(id string) ([]Profile, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	if len(s.config.Profiles) <= 1 { return nil, errors.New("at least one connection profile must remain") }
	index := -1
	for i, p := range s.config.Profiles { if p.ID == id { index = i; break } }
	if index < 0 { return nil, errors.New("profile not found") }
	s.config.Profiles = append(s.config.Profiles[:index], s.config.Profiles[index+1:]...)
	if s.config.ActiveProfile == id { s.config.ActiveProfile = s.config.Profiles[0].ID }
	if err := s.saveConfigLocked(); err != nil { return nil, err }
	return append([]Profile(nil), s.config.Profiles...), nil
}

func (s *AppService) GetSettings() AppSettings {
	s.mu.RLock(); defer s.mu.RUnlock()
	return s.config.Settings
}

func (s *AppService) SaveSettings(settings AppSettings) error {
	if settings.EnginePath != "" {
		if _, err := inspectEngine(settings.EnginePath); err != nil { return err }
	}
	s.mu.Lock(); defer s.mu.Unlock()
	s.config.Settings = settings
	return s.saveConfigLocked()
}

func validateProfile(p Profile) error {
	if strings.TrimSpace(p.Name) == "" { return errors.New("profile name is required") }
	if len(p.Name) > 64 { return errors.New("profile name must be 64 characters or fewer") }
	host, portText, err := net.SplitHostPort(strings.TrimSpace(p.LocalTCP))
	if err != nil || host == "" { return errors.New("local TCP target must be in host:port format, e.g. 127.0.0.1:19132") }
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 { return errors.New("local TCP port must be between 1 and 65535") }
	if p.LocalUDPPort < 1 || p.LocalUDPPort > 65535 { return errors.New("local UDP fallback port must be between 1 and 65535") }
	if p.LocalUDPIP != "" && net.ParseIP(p.LocalUDPIP) == nil { return errors.New("local UDP IP must be a valid IPv4 or IPv6 address") }
	region := strings.ToUpper(strings.TrimSpace(p.Region))
	switch region {
	case "", "AUTO", "US", "TX", "UK", "BR", "ASIA": return nil
	default:
		if strings.ContainsAny(region, " 	
/") || len(region) > 255 { return errors.New("region must be AUTO, a known region code, or a host/IP without spaces") }
		return nil
	}
}

func profileArgs(p Profile) []string {
	args := []string{"-local-http", p.LocalTCP, "-local-udp-port", strconv.Itoa(p.LocalUDPPort)}
	region := strings.TrimSpace(p.Region)
	if region != "" && !strings.EqualFold(region, "AUTO") { args = append(args, "-node", region) }
	if p.LocalUDPIP != "" { args = append(args, "-local-udp-ip", p.LocalUDPIP) }
	return args
}

func (s *AppService) resolveEngine() string {
	s.mu.RLock(); configured := s.config.Settings.EnginePath; s.mu.RUnlock()
	candidates := []string{}
	if configured != "" { candidates = append(candidates, configured) }
	exe, _ := os.Executable()
	if exe != "" {
		name := "nethernet-client"
		if runtime.GOOS == "windows" { name += ".exe" }
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	name := "nethernet-client"
	if runtime.GOOS == "windows" { name += ".exe" }
	if found, err := exec.LookPath(name); err == nil { candidates = append(candidates, found) }
	candidates = append(candidates, filepath.Join(s.configDir, "bin", name))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() { return candidate }
	}
	return ""
}

func inspectEngine(path string) (EngineInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil { return EngineInfo{}, err }
	info, err := os.Stat(absolute)
	if err != nil { return EngineInfo{}, fmt.Errorf("engine binary not found: %w", err) }
	if info.IsDir() { return EngineInfo{}, errors.New("engine path points to a directory") }
	out, err := exec.Command(absolute, "-version").CombinedOutput()
	if err != nil { return EngineInfo{}, fmt.Errorf("could not run engine -version: %s", strings.TrimSpace(string(out))) }
	version := strings.TrimSpace(string(out))
	if version == "" || len(version) > 80 { return EngineInfo{}, errors.New("engine returned an invalid version string") }
	return EngineInfo{Path:absolute, Version:version}, nil
}

func (s *AppService) InstallOrUpdateEngine() (EngineInfo, error) {
	client := &http.Client{Timeout:90*time.Second}
	req, err := http.NewRequest(http.MethodGet, releaseAPI, nil)
	if err != nil { return EngineInfo{}, err }
	req.Header.Set("User-Agent", "Nethernet-Desktop")
	resp, err := client.Do(req)
	if err != nil { return EngineInfo{}, fmt.Errorf("could not query latest release: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return EngineInfo{}, fmt.Errorf("latest release lookup returned HTTP %d", resp.StatusCode) }
	var release struct {
		TagName string \`json:"tag_name"\`
		Assets []struct {
			Name string \`json:"name"\`
			URL string \`json:"browser_download_url"\`
		} \`json:"assets"\`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil { return EngineInfo{}, fmt.Errorf("invalid release metadata: %w", err) }
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" { return EngineInfo{}, fmt.Errorf("unsupported engine architecture: %s", arch) }
	asset := "nethernet-client-" + runtime.GOOS + "-" + arch
	if runtime.GOOS == "windows" { asset += ".exe" }
	assetURL := ""
	for _, item := range release.Assets { if item.Name == asset { assetURL = item.URL; break } }
	if assetURL == "" { return EngineInfo{}, fmt.Errorf("release %s does not contain %s", release.TagName, asset) }

	download := func(url string, limit int64) ([]byte, error) {
		rq, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil { return nil, err }
		rq.Header.Set("User-Agent", "Nethernet-Desktop")
		r, err := client.Do(rq)
		if err != nil { return nil, err }
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK { return nil, fmt.Errorf("download returned HTTP %d", r.StatusCode) }
		return io.ReadAll(io.LimitReader(r.Body, limit+1))
	}
	binaryData, err := download(assetURL, 100<<20)
	if err != nil { return EngineInfo{}, fmt.Errorf("engine download failed: %w", err) }
	if len(binaryData) == 0 || len(binaryData) > 100<<20 { return EngineInfo{}, errors.New("downloaded engine has an invalid size") }
	checksumsURL := strings.TrimSuffix(assetURL, asset) + "SHA256SUMS"
	checksumData, err := download(checksumsURL, 1<<20)
	if err != nil { return EngineInfo{}, fmt.Errorf("could not download SHA256SUMS: %w", err) }
	expected := checksumForAsset(string(checksumData), asset)
	if expected == "" { return EngineInfo{}, fmt.Errorf("release checksum manifest has no entry for %s", asset) }
	sum := sha256.Sum256(binaryData)
	if !strings.EqualFold(expected, hex.EncodeToString(sum[:])) { return EngineInfo{}, errors.New("engine SHA-256 verification failed; downloaded file was not installed") }

	destDir := filepath.Join(s.configDir, "bin")
	if err := os.MkdirAll(destDir, 0700); err != nil { return EngineInfo{}, err }
	dest := filepath.Join(destDir, asset)
	tmp := dest + ".download"
	if err := os.WriteFile(tmp, binaryData, 0700); err != nil { return EngineInfo{}, err }
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmp, 0755); err != nil { _ = os.Remove(tmp); return EngineInfo{}, err }
	}
	if err := os.Rename(tmp, dest); err != nil { _ = os.Remove(tmp); return EngineInfo{}, err }
	engine, err := inspectEngine(dest)
	if err != nil { return EngineInfo{}, err }
	s.mu.Lock(); s.config.Settings.EnginePath = dest; err = s.saveConfigLocked(); s.mu.Unlock()
	if err != nil { return EngineInfo{}, err }
	engine.Updated = true
	s.addLog("INFO", "Installed tunnel engine "+engine.Version)
	return engine, nil
}

func checksumForAsset(manifest, asset string) string {
	for _, line := range strings.Split(manifest, "
") {
		fields := strings.Fields(line)
		if len(fields) < 2 { continue }
		if strings.TrimPrefix(fields[1], "*") == asset && len(fields[0]) == 64 {
			if _, err := hex.DecodeString(fields[0]); err == nil { return strings.ToLower(fields[0]) }
		}
	}
	return ""
}

func (s *AppService) StartTunnel() error {
	s.mu.Lock()
	if s.cmd != nil { s.mu.Unlock(); return errors.New("tunnel process is already running") }
	profile := s.activeProfileLocked()
	s.mu.Unlock()
	if active, _ := backgroundStatus(); active { return errors.New("background tunnel is already running; stop it before starting the desktop-managed process") }
	binary := s.resolveEngine()
	if binary == "" { return errors.New("tunnel engine is not installed; use Settings → Install / Update Engine first") }
	if _, err := inspectEngine(binary); err != nil { return err }
	cmd := exec.Command(binary, profileArgs(profile)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil { return err }
	stderr, err := cmd.StderrPipe()
	if err != nil { return err }
	if err := cmd.Start(); err != nil { return fmt.Errorf("could not start tunnel engine: %w", err) }

	done := make(chan struct{})
	s.mu.Lock()
	if s.cmd != nil { s.mu.Unlock(); _ = cmd.Process.Kill(); return errors.New("another tunnel process started concurrently") }
	s.cmd, s.done, s.startedAt = cmd, done, time.Now()
	s.connection, s.lastError = "Starting", ""
	s.mu.Unlock()
	s.addLog("INFO", "Tunnel engine started with profile "+profile.Name)

	stdoutDone, stderrDone := make(chan struct{}), make(chan struct{})
	go s.captureOutput(stdout, stdoutDone)
	go s.captureOutput(stderr, stderrDone)
	go func() {
		err := cmd.Wait()
		<-stdoutDone
		<-stderrDone
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
			if err != nil { s.lastError = err.Error() }
			s.connection = "Stopped"
			s.addLogLocked("INFO", "Tunnel engine stopped")
		}
		s.mu.Unlock()
		close(done)
	}()
	return nil
}

func (s *AppService) captureOutput(reader io.Reader, finished chan<- struct{}) {
	defer close(finished)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := cleanLogLine(scanner.Text())
		if line == "" { continue }
		lower, level := strings.ToLower(line), "INFO"
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") { level = "ERROR" } else if strings.Contains(lower, "warn") { level = "WARN" }
		s.addLog(level, line)
	}
	if err := scanner.Err(); err != nil { s.addLog("WARN", "Could not read all tunnel output: "+err.Error()) }
}

func cleanLogLine(line string) string {
	line = ansiPattern.ReplaceAllString(line, "")
	line = strings.TrimSpace(secretPattern.ReplaceAllString(strings.ReplaceAll(line, "", ""), "$1[redacted]"))
	if line == "" { return "" }
	lower := strings.ToLower(line)
	relevant := []string{"[tunnel client]", "[bedrock node tunnel]", "authenticated tunnel", "claim", "reconnect", "disconnected", "error", "warning", "fast-lane", "already running"}
	keep := false
	for _, marker := range relevant { if strings.Contains(lower, marker) { keep = true; break } }
	if !keep { return "" }
	if len(line) > 600 { line = line[:600] + "…" }
	return line
}

func (s *AppService) addLog(level, message string) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.addLogLocked(level, message)
}

func (s *AppService) addLogLocked(level, message string) {
	s.nextLogID++
	s.logs = append(s.logs, LogEntry{ID:s.nextLogID, Time:time.Now().Format("15:04:05"), Level:level, Message:message})
	if len(s.logs) > 500 { s.logs = append([]LogEntry(nil), s.logs[len(s.logs)-500:]...) }
	lower := strings.ToLower(message)
	if strings.Contains(lower, "authenticated tunnel established") { s.connection = "Connected" } else if strings.Contains(lower, "claim") { s.connection = "Waiting for Claim" } else if strings.Contains(lower, "reconnect") || strings.Contains(lower, "disconnected") { s.connection = "Reconnecting" }
}

func (s *AppService) GetLogs() []LogEntry {
	s.mu.RLock(); defer s.mu.RUnlock()
	return append([]LogEntry(nil), s.logs...)
}

func (s *AppService) ClearLogs() {
	s.mu.Lock(); defer s.mu.Unlock()
	s.logs = nil
}

func (s *AppService) StopTunnel() error {
	if active, _ := backgroundStatus(); active { return stopBackground() }
	s.mu.RLock(); cmd, done := s.cmd, s.done; s.mu.RUnlock()
	if cmd == nil || cmd.Process == nil { return nil }
	if err := cmd.Process.Signal(os.Interrupt); err != nil { _ = cmd.Process.Kill() }
	if done != nil {
		select {
		case <-done:
		case <-time.After(4*time.Second): _ = cmd.Process.Kill(); <-done
		}
	}
	return nil
}

func (s *AppService) RestartTunnel() error {
	if active, _ := backgroundStatus(); active {
		binary := s.resolveEngine()
		if binary == "" { return errors.New("tunnel engine is not installed") }
		s.mu.RLock(); p := s.activeProfileLocked(); s.mu.RUnlock()
		if err := configureBackground(binary, profileArgs(p)); err != nil { return err }
		return restartBackground()
	}
	_ = s.StopTunnel()
	return s.StartTunnel()
}

func (s *AppService) StartBackground() error {
	s.mu.RLock(); p, running := s.activeProfileLocked(), s.cmd != nil; s.mu.RUnlock()
	if running { return errors.New("stop the foreground tunnel before enabling background mode") }
	binary := s.resolveEngine()
	if binary == "" { return errors.New("tunnel engine is not installed; use Settings → Install / Update Engine first") }
	if _, err := inspectEngine(binary); err != nil { return err }
	if err := configureBackground(binary, profileArgs(p)); err != nil { return err }
	if err := startBackground(); err != nil { return err }
	s.addLog("INFO", "Background tunnel service started")
	return nil
}

func (s *AppService) StopBackground() error {
	if err := stopBackground(); err != nil { return err }
	s.addLog("INFO", "Background tunnel service stopped")
	return nil
}

func (s *AppService) RemoveBackground() error {
	if err := removeBackground(); err != nil { return err }
	s.addLog("INFO", "Background tunnel service removed")
	return nil
}

func (s *AppService) GetSnapshot() AppSnapshot {
	s.mu.RLock()
	cmd, started, connection, lastError := s.cmd, s.startedAt, s.connection, s.lastError
	profile, profiles := s.activeProfileLocked(), append([]Profile(nil), s.config.Profiles...)
	s.mu.RUnlock()
	background, _ := backgroundStatus()
	engine := s.resolveEngine()
	engineVersion, engineFound := "", false
	if engine != "" {
		if info, err := inspectEngine(engine); err == nil { engineFound, engineVersion = true, info.Version }
	}
	running, mode, uptime, startedText := cmd != nil, "Stopped", "—", ""
	if background { mode, connection = "Background service", "Background service active" } else if running {
		mode, uptime, startedText = "Desktop process", time.Since(started).Round(time.Second).String(), started.Format(time.RFC3339)
	}
	if !running && !background { connection = "Stopped" }
	return AppSnapshot{
		AppVersion:desktopVersion, EngineFound:engineFound, EnginePath:engine, EngineVersion:engineVersion,
		Running:running, BackgroundRunning:background, BackgroundSupported:backgroundSupported(),
		Mode:mode, ConnectionState:connection, Uptime:uptime, StartedAt:startedText, LastError:lastError,
		ActiveProfile:profile, Profiles:profiles,
	}
}

func (s *AppService) RunDiagnostics() []DiagnosticResult {
	snapshot := s.GetSnapshot()
	results := make([]DiagnosticResult, 0, 5)
	if snapshot.EngineFound { results = append(results, DiagnosticResult{"Tunnel engine","ok","Found "+snapshot.EngineVersion+" at "+snapshot.EnginePath}) } else {
		results = append(results, DiagnosticResult{"Tunnel engine","fail","No usable nethernet-client binary found. Install it from Settings or set its path."})
	}
	p := snapshot.ActiveProfile
	conn, err := net.DialTimeout("tcp", p.LocalTCP, 900*time.Millisecond)
	if err == nil { _ = conn.Close(); results = append(results, DiagnosticResult{"Local Bedrock TCP","ok",p.LocalTCP+" accepted a TCP connection."}) } else {
		results = append(results, DiagnosticResult{"Local Bedrock TCP","warn",p.LocalTCP+" is not accepting connections. Start the Bedrock Dedicated Server before hosting."})
	}
	if address := controlAddress(p.Region); address != "" {
		conn, err := net.DialTimeout("tcp", address, 2500*time.Millisecond)
		if err == nil { _ = conn.Close(); results = append(results, DiagnosticResult{"Relay control endpoint","ok",address+" accepted a TCP connection. This does not verify the authenticated UDP fast-lane."}) } else {
			results = append(results, DiagnosticResult{"Relay control endpoint","warn","Could not reach "+address+" over TCP: "+err.Error()})
		}
	} else { results = append(results, DiagnosticResult{"Relay control endpoint","info","Automatic or custom region selected; endpoint probing is not available for that selection."}) }
	client := &http.Client{Timeout:5*time.Second}
	resp, err := client.Get("https://bedrocknode.net/api/nodes")
	if err != nil { results = append(results, DiagnosticResult{"Bedrock Node service API","warn","Could not reach bedrocknode.net: "+err.Error()}) } else {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 { results = append(results, DiagnosticResult{"Bedrock Node service API","ok","Service API responded with HTTP "+strconv.Itoa(resp.StatusCode)+"."}) } else {
			results = append(results, DiagnosticResult{"Bedrock Node service API","warn","Service API returned HTTP "+strconv.Itoa(resp.StatusCode)+"."})
		}
	}
	return results
}

func controlAddress(region string) string {
	controls := map[string]string{
		"US":"51.222.142.179:7000","TX":"216.128.129.60:7000",
		"UK":"104.238.173.37:7000","BR":"216.128.170.42:7000","ASIA":"45.32.121.108:7000",
	}
	return controls[strings.ToUpper(strings.TrimSpace(region))]
}

func (s *AppService) ServiceShutdown() error {
	active, _ := backgroundStatus()
	if active {
		return nil
	}
	return s.StopTunnel()
}
