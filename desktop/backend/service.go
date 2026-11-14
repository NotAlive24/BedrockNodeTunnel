package backend

import (
 "bufio"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/url"
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

const unitName = "nethernet-desktop-tunnel.service"
const legacyUnit = "bedrocknode-tunnel.service"
const windowsTaskName = "Nethernet Desktop Tunnel"
var tokenRE = regexp.MustCompile("(?i)(token[=: ]+)[^\\s&]+")

type ConnectionProfile struct {
 Name string `json:"name"`
 ClientPath string `json:"clientPath"`
 Node string `json:"node"`
 ControlAddr string `json:"controlAddr"`
 WebURL string `json:"webUrl"`
 LocalTCP string `json:"localTcp"`
 LocalUDPIP string `json:"localUdpIp"`
 LocalUDPPort int `json:"localUdpPort"`
}
type Settings struct {
 Profiles []ConnectionProfile `json:"profiles,omitempty"`
 ProfileName string `json:"profileName"`
 ClientPath string `json:"clientPath"`
 Node string `json:"node"`
 ControlAddr string `json:"controlAddr"`
 WebURL string `json:"webUrl"`
 LocalTCP string `json:"localTcp"`
 LocalUDPIP string `json:"localUdpIp"`
 LocalUDPPort int `json:"localUdpPort"`
 Theme string `json:"theme"`
 Accent string `json:"accent"`
 CloseToTray bool `json:"closeToTray"`
 LaunchAtLogin bool `json:"launchAtLogin"`
 StartMinimized bool `json:"startMinimized"`
}
type LogEntry struct { Time string `json:"time"`; Level string `json:"level"`; Message string `json:"message"` }
type Snapshot struct {
 State string `json:"state"`; Message string `json:"message"`; ClientPath string `json:"clientPath"`
 ProfileName string `json:"profileName"`; Node string `json:"node"`; LocalTCP string `json:"localTcp"`; StartedAt string `json:"startedAt"`
 BackgroundSupported bool `json:"backgroundSupported"`; BackgroundInstalled bool `json:"backgroundInstalled"`
 BackgroundEnabled bool `json:"backgroundEnabled"`; BackgroundActive bool `json:"backgroundActive"`
 LegacyServiceActive bool `json:"legacyServiceActive"`; LegacyServiceEnabled bool `json:"legacyServiceEnabled"`
 BackgroundNeedsRestart bool `json:"backgroundNeedsRestart"`
 AutostartSupported bool `json:"autostartSupported"`
}
type Service struct {
 mu sync.Mutex
 settings Settings
 cmd *exec.Cmd
 done chan struct{}
 starting bool
 stopRequested bool
 state string
 message string
 clientPath string
 startedAt time.Time
 logs []LogEntry
 backgroundNeedsRestart bool
 loadWarning string
}
func defaults() Settings { return Settings{ProfileName:"Default profile",LocalTCP:"127.0.0.1:19132",LocalUDPPort:47334,Theme:"dark",Accent:"cyan",CloseToTray:true} }
func NewService() *Service {
 s:=&Service{settings:defaults(),state:"stopped",message:"The tunnel is not running."}
 s.settings.Profiles=[]ConnectionProfile{profileFromSettings(s.settings)}
 p,e:=settingsPath(); if e!=nil{s.loadWarning=e.Error();return s}
 b,e:=os.ReadFile(p);if errors.Is(e,os.ErrNotExist){return s};if e!=nil{s.loadWarning=e.Error();return s}
 var v Settings;if e=json.Unmarshal(b,&v);e!=nil{s.loadWarning=e.Error();return s}
 v,e=validate(v);if e!=nil{s.loadWarning=e.Error();return s};if len(v.Profiles)==0{v.Profiles=[]ConnectionProfile{profileFromSettings(v)}}else{v.Profiles=upsertProfile(v.Profiles,profileFromSettings(v))};s.settings=v;return s
}
func cloneProfiles(items []ConnectionProfile) []ConnectionProfile { return append([]ConnectionProfile(nil),items...) }
func profileFromSettings(v Settings) ConnectionProfile { return ConnectionProfile{Name:v.ProfileName,ClientPath:v.ClientPath,Node:v.Node,ControlAddr:v.ControlAddr,WebURL:v.WebURL,LocalTCP:v.LocalTCP,LocalUDPIP:v.LocalUDPIP,LocalUDPPort:v.LocalUDPPort} }
func applyProfile(v Settings,p ConnectionProfile) Settings { v.ProfileName=p.Name;v.ClientPath=p.ClientPath;v.Node=p.Node;v.ControlAddr=p.ControlAddr;v.WebURL=p.WebURL;v.LocalTCP=p.LocalTCP;v.LocalUDPIP=p.LocalUDPIP;v.LocalUDPPort=p.LocalUDPPort;return v }
func upsertProfile(items []ConnectionProfile,p ConnectionProfile) []ConnectionProfile { out:=cloneProfiles(items);for i:=range out{if strings.EqualFold(out[i].Name,p.Name){out[i]=p;return out}};return append(out,p) }
func (s *Service) GetSettings() Settings { s.mu.Lock();defer s.mu.Unlock();v:=s.settings;v.Profiles=cloneProfiles(v.Profiles);return v }
func (s *Service) GetProfiles() []ConnectionProfile { s.mu.Lock();defer s.mu.Unlock();return cloneProfiles(s.settings.Profiles) }
func (s *Service) SaveProfile(name string,v Settings)(Settings,error) { name=strings.TrimSpace(name);if name==""{return Settings{},errors.New("profile name is required")};if strings.ContainsAny(name,"\\x00\\r\\n"){return Settings{},errors.New("profile name contains invalid characters")};s.mu.Lock();if len(v.Profiles)==0{v.Profiles=cloneProfiles(s.settings.Profiles)};s.mu.Unlock();v.ProfileName=name;return s.SaveSettings(v) }
func (s *Service) SwitchProfile(name string)(Settings,error) { s.mu.Lock();v:=s.settings;profiles:=cloneProfiles(v.Profiles);s.mu.Unlock();for _,p:=range profiles{if strings.EqualFold(p.Name,name){v=applyProfile(v,p);v.Profiles=profiles;return s.SaveSettings(v)}};return Settings{},fmt.Errorf("profile %q not found",name) }
func (s *Service) DeleteProfile(name string)([]ConnectionProfile,error) { s.mu.Lock();v:=s.settings;profiles:=cloneProfiles(v.Profiles);s.mu.Unlock();if len(profiles)<=1{return profiles,errors.New("the last connection profile cannot be deleted")};index:=-1;for i,p:=range profiles{if strings.EqualFold(p.Name,name){index=i;break}};if index<0{return profiles,fmt.Errorf("profile %q not found",name)};profiles=append(profiles[:index],profiles[index+1:]...);if strings.EqualFold(v.ProfileName,name){v=applyProfile(v,profiles[0])};v.Profiles=profiles;saved,e:=s.SaveSettings(v);if e!=nil{return profiles,e};return saved.Profiles,nil }
func validate(v Settings)(Settings,error) {
 d:=defaults();v.ProfileName=strings.TrimSpace(v.ProfileName);if v.ProfileName==""{v.ProfileName=d.ProfileName}
 v.ClientPath=strings.TrimSpace(v.ClientPath);v.Node=strings.TrimSpace(v.Node);v.ControlAddr=strings.TrimSpace(v.ControlAddr);v.WebURL=strings.TrimSpace(v.WebURL);v.LocalTCP=strings.TrimSpace(v.LocalTCP);v.LocalUDPIP=strings.TrimSpace(v.LocalUDPIP)
 if v.LocalTCP==""{v.LocalTCP=d.LocalTCP};if v.LocalUDPPort==0{v.LocalUDPPort=d.LocalUDPPort};if v.Theme==""{v.Theme=d.Theme};if v.Accent==""{v.Accent=d.Accent}
 for k,x:=range map[string]string{"profile":v.ProfileName,"path":v.ClientPath,"node":v.Node,"control":v.ControlAddr,"url":v.WebURL,"tcp":v.LocalTCP,"udp":v.LocalUDPIP}{if strings.ContainsAny(x,"\x00\r\n"){return Settings{},fmt.Errorf("%s has invalid characters",k)}}
 h,p,e:=net.SplitHostPort(v.LocalTCP);if e!=nil||h==""{return Settings{},errors.New("local TCP target must be host:port")};n,e:=strconv.Atoi(p);if e!=nil||n<1||n>65535{return Settings{},errors.New("local TCP port must be 1-65535")}
 if v.LocalUDPPort<1||v.LocalUDPPort>65535{return Settings{},errors.New("local UDP port must be 1-65535")}
 if v.LocalUDPIP!=""&&net.ParseIP(v.LocalUDPIP)==nil{return Settings{},errors.New("invalid local UDP IP")}
 if v.Node!=""&&strings.ContainsAny(v.Node," \t"){return Settings{},errors.New("node must be one region code or host")}
 if v.ControlAddr!=""{h,p,e=net.SplitHostPort(v.ControlAddr);if e!=nil||h==""{return Settings{},errors.New("control address must be host:port")};n,e=strconv.Atoi(p);if e!=nil||n<1||n>65535{return Settings{},errors.New("control port must be 1-65535")}}
 if v.WebURL!=""{u,e:=url.Parse(v.WebURL);if e!=nil||(u.Scheme!="http"&&u.Scheme!="https")||u.Host==""||u.User!=nil{return Settings{},errors.New("dashboard URL must be HTTP(S) without credentials")}}
 if v.Theme!="dark"&&v.Theme!="light"&&v.Theme!="system"{return Settings{},errors.New("invalid theme")}
 switch v.Accent{case "cyan","mint","amber","violet":default:return Settings{},errors.New("invalid accent")}
 return v,nil
}
func settingsPath()(string,error){d,e:=os.UserConfigDir();if e!=nil{return "",e};return filepath.Join(d,"bedrocknode-tunnel","desktop.json"),nil}
func saveJSON(p string,v any)error{if e:=os.MkdirAll(filepath.Dir(p),0700);e!=nil{return e};b,e:=json.MarshalIndent(v,"","  ");if e!=nil{return e};f,e:=os.CreateTemp(filepath.Dir(p),".desktop-*.tmp");if e!=nil{return e};name:=f.Name();defer os.Remove(name);if e=f.Chmod(0600);e==nil{_,e=f.Write(append(b,'\n'))};if e==nil{e=f.Sync()};ce:=f.Close();if e!=nil{return e};if ce!=nil{return ce};return os.Rename(name,p)}
func (s *Service) SaveSettings(v Settings)(Settings,error) {
 validated,e:=validate(v);if e!=nil{return Settings{},e}
 s.mu.Lock();if len(validated.Profiles)==0{validated.Profiles=cloneProfiles(s.settings.Profiles)};validated.Profiles=upsertProfile(validated.Profiles,profileFromSettings(validated));s.mu.Unlock()
 path,e:=settingsPath();if e!=nil{return Settings{},e};if e=saveJSON(path,validated);e!=nil{return Settings{},e}
 s.mu.Lock();s.settings=validated;s.mu.Unlock()
 if backgroundInstalled(){if e=updateBackground(validated);e!=nil{return validated,e};s.mu.Lock();s.backgroundNeedsRestart=true;s.mu.Unlock()}
 return validated,nil
}
func (s *Service) SetStartupOptions(enabled, startMinimized bool) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.settings
	next := old
	next.LaunchAtLogin = enabled
	next.StartMinimized = startMinimized
	if err := configureAutostart(enabled, startMinimized); err != nil { return old, err }
	path, err := settingsPath()
	if err == nil { err = saveJSON(path, next) }
	if err != nil {
		if rollbackErr := configureAutostart(old.LaunchAtLogin, old.StartMinimized); rollbackErr != nil {
			return old, fmt.Errorf("save startup settings: %w (also failed to restore previous autostart state: %v)", err, rollbackErr)
		}
		return old, err
	}
	s.settings = next
	return next, nil
}

func (s *Service) GetSnapshot() Snapshot {
 s.mu.Lock();v:=s.settings;running:=s.cmd!=nil||s.starting;x:=Snapshot{State:s.state,Message:s.message,ClientPath:s.clientPath,ProfileName:v.ProfileName,Node:v.Node,LocalTCP:v.LocalTCP,BackgroundSupported:backgroundSupported(),AutostartSupported:runtime.GOOS=="linux"||runtime.GOOS=="windows",BackgroundNeedsRestart:s.backgroundNeedsRestart};if x.Node==""{x.Node="Automatic"};if !s.startedAt.IsZero(){x.StartedAt=s.startedAt.Format(time.RFC3339)};if x.ClientPath==""{x.ClientPath,_=resolve(v)};warn:=s.loadWarning;s.mu.Unlock()
 if x.BackgroundSupported{x.BackgroundInstalled=backgroundInstalled();x.BackgroundEnabled=backgroundEnabled();x.BackgroundActive=backgroundActive();x.LegacyServiceActive=legacyBackgroundActive();x.LegacyServiceEnabled=legacyBackgroundEnabled()}
 if !running&&x.BackgroundActive{x.State="background-active";x.Message="Background client task is active; relay authentication is not confirmed by the service manager."}else if !running&&warn!=""{x.Message=warn};return x
}
func (s *Service) StartClient()(Snapshot,error) {
 s.mu.Lock();if s.cmd!=nil||s.starting{s.mu.Unlock();return s.GetSnapshot(),errors.New("client already running or starting")};v:=s.settings;s.starting=true;s.state="starting";s.message="Preparing client";s.mu.Unlock()
 fail:=func(e error)(Snapshot,error){s.mu.Lock();s.starting=false;s.state="error";s.message=e.Error();s.logLocked("error",e.Error());s.mu.Unlock();return s.GetSnapshot(),e}
 if backgroundActive(){return fail(errors.New("background tunnel is active; stop it before starting a foreground client"))}
 if legacyBackgroundActive()||legacyBackgroundEnabled(){return fail(errors.New("an older Bedrock Node Tunnel background service is active or enabled; disable it to avoid duplicate tunnels"))}
 bin,e:=resolve(v);if e!=nil{return fail(e)};cmd:=exec.Command(bin,clientArgs(v)...);out,e:=cmd.StdoutPipe();if e!=nil{return fail(e)};er,e:=cmd.StderrPipe();if e!=nil{out.Close();return fail(e)};if e=cmd.Start();e!=nil{out.Close();er.Close();return fail(e)}
 done:=make(chan struct{});s.mu.Lock();s.cmd=cmd;s.done=done;s.starting=false;s.stopRequested=false;s.startedAt=time.Now();s.clientPath=bin;s.logs=nil;s.state="connecting";s.message="Client started; waiting for relay authentication.";s.mu.Unlock()
 var wg sync.WaitGroup;wg.Add(2);go func(){defer wg.Done();s.readLogs(out)}();go func(){defer wg.Done();s.readLogs(er)}();go func(){wg.Wait();e:=cmd.Wait();s.mu.Lock();defer s.mu.Unlock();defer close(done);if s.cmd==cmd{s.cmd=nil;s.done=nil;s.startedAt=time.Time{};if s.stopRequested{s.state="stopped";s.message="Client stopped."}else if e!=nil{s.state="error";s.message="Client exited: "+e.Error();s.logLocked("error",s.message)}else{s.state="stopped";s.message="Client exited."}}}();return s.GetSnapshot(),nil
}
func (s *Service) StopClient()(Snapshot,error){s.mu.Lock();c,d:=s.cmd,s.done;if s.starting&&c==nil{s.mu.Unlock();return s.GetSnapshot(),errors.New("client is still starting")};if c==nil{s.mu.Unlock();return s.GetSnapshot(),nil};s.stopRequested=true;s.state="stopping";s.mu.Unlock();if c.Process!=nil{if e:=c.Process.Kill();e!=nil&&!errors.Is(e,os.ErrProcessDone){return s.GetSnapshot(),e}};select{case<-d:case<-time.After(5*time.Second):return s.GetSnapshot(),errors.New("client did not exit in time")};return s.GetSnapshot(),nil}
func (s *Service) readLogs(r io.Reader){sc:=bufio.NewScanner(r);sc.Buffer(make([]byte,32768),1024*1024);for sc.Scan(){line:=sanitize(sc.Text());if line==""{continue};low:=strings.ToLower(line);level:="info";if strings.Contains(low,"error")||strings.Contains(low,"failed"){level="error"}else if strings.Contains(low,"warning")||strings.Contains(low,"retry"){level="warning"};s.mu.Lock();s.logLocked(level,line);switch{case strings.Contains(low,"authenticated tunnel established"):s.state="connected";s.message="Relay authentication succeeded.";case strings.Contains(low,"tunnel disconnected"):s.state="reconnecting";s.message="Relay disconnected; client is retrying.";case strings.Contains(low,"claim code:"):s.state="waiting-for-claim";s.message="Claim the tunnel in the dashboard."};s.mu.Unlock()}}
func sanitize(x string)string{return tokenRE.ReplaceAllString(strings.TrimSpace(x),"$1[REDACTED]")}
func (s *Service) logLocked(level,msg string){s.logs=append(s.logs,LogEntry{time.Now().Format(time.RFC3339),level,sanitize(msg)});if len(s.logs)>300{s.logs=append([]LogEntry(nil),s.logs[len(s.logs)-300:]...)}}
func (s *Service) GetLogs()[]LogEntry{s.mu.Lock();running:=s.cmd!=nil;out:=append([]LogEntry(nil),s.logs...);s.mu.Unlock();if running{return out};if runtime.GOOS=="linux"&&unitActive(unitName){ctx,c:=context.WithTimeout(context.Background(),4*time.Second);defer c();b,e:=exec.CommandContext(ctx,"journalctl","--user","-u",unitName,"-n","150","--no-pager","-o","short-iso").Output();if e==nil{var a []LogEntry;for _,line:=range strings.Split(strings.TrimSpace(string(b)),"\n"){if line!=""{a=append(a,LogEntry{"journal","info",sanitize(line)})}};return a}};return out}
func (s *Service) ServiceShutdown()error{_,e:=s.StopClient();return e}
func clientArgs(v Settings)[]string{a:=[]string{};if v.Node!=""{a=append(a,"-node",v.Node)};if v.ControlAddr!=""{a=append(a,"-control",v.ControlAddr)};if v.WebURL!=""{a=append(a,"-web",v.WebURL)};a=append(a,"-local-http",v.LocalTCP,"-local-udp-port",strconv.Itoa(v.LocalUDPPort));if v.LocalUDPIP!=""{a=append(a,"-local-udp-ip",v.LocalUDPIP)};return a}
func resolve(v Settings)(string,error){p:=v.ClientPath;if p==""{n:="nethernet-client";if runtime.GOOS=="windows"{n+=".exe";if d:=os.Getenv("LOCALAPPDATA");d!=""{q:=filepath.Join(d,"Programs","BedrockNodeTunnel",n);if fileExists(q){p=q}}}else if h,e:=os.UserHomeDir();e==nil{q:=filepath.Join(h,".local","bin",n);if fileExists(q){p=q}};if p==""{var e error;p,e=exec.LookPath(n);if e!=nil{return "",errors.New("nethernet-client not found; install it or set its path")}}};p,e:=filepath.Abs(p);if e!=nil{return "",e};i,e:=os.Stat(p);if e!=nil{return "",e};if i.IsDir(){return "",errors.New("client path is a directory")};if runtime.GOOS!="windows"&&i.Mode()&0111==0{return "",errors.New("client file is not executable")};return p,nil}
func unitPath()string{d,e:=os.UserConfigDir();if e!=nil{return ""};return filepath.Join(d,"systemd","user",unitName)}
func renderUnit(bin string,args []string)string{all:=append([]string{bin},args...);for i,x:=range all{x=strings.ReplaceAll(x,"\\","\\\\");x=strings.ReplaceAll(x,"\"","\\\"");x=strings.ReplaceAll(x,"$","$$");x=strings.ReplaceAll(x,"%","%%");all[i]="\""+x+"\""};return "[Unit]\nDescription=Bedrock Node Tunnel client (Nethernet Desktop)\nAfter=default.target\n\n[Service]\nType=simple\nExecStart="+strings.Join(all," ")+"\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n"}
func (s *Service) writeUnit(v Settings)error{bin,e:=resolve(v);if e!=nil{return e};p:=unitPath();if e=os.MkdirAll(filepath.Dir(p),0700);e!=nil{return e};tmp:=p+".tmp";if e=os.WriteFile(tmp,[]byte(renderUnit(bin,clientArgs(v))),0600);e!=nil{return e};return os.Rename(tmp,p)}
func (s *Service) EnableBackgroundService()(Snapshot,error) {
 if !backgroundSupported(){return s.GetSnapshot(),errors.New("background tunnel management is unsupported on this system")}
 s.mu.Lock();busy:=s.cmd!=nil||s.starting;v:=s.settings;s.mu.Unlock()
 if busy{return s.GetSnapshot(),errors.New("stop the foreground client first")}
 if legacyBackgroundActive()||legacyBackgroundEnabled(){return s.GetSnapshot(),errors.New("older bedrocknode-tunnel service conflict; stop and disable it first")}
 if e:=installBackground(v);e!=nil{return s.GetSnapshot(),e};if e:=startBackground();e!=nil{return s.GetSnapshot(),e}
 s.mu.Lock();s.backgroundNeedsRestart=false;s.mu.Unlock();return s.GetSnapshot(),nil
}
func (s *Service) RestartBackgroundService()(Snapshot,error) {
 if !backgroundSupported()||!backgroundInstalled(){return s.GetSnapshot(),errors.New("desktop background service is not installed")}
 if legacyBackgroundActive()||legacyBackgroundEnabled(){return s.GetSnapshot(),errors.New("older bedrocknode-tunnel service conflict; stop and disable it first")}
 s.mu.Lock();v:=s.settings;s.mu.Unlock();if e:=updateBackground(v);e!=nil{return s.GetSnapshot(),e};if e:=restartBackground();e!=nil{return s.GetSnapshot(),e}
 s.mu.Lock();s.backgroundNeedsRestart=false;s.mu.Unlock();return s.GetSnapshot(),nil
}
func (s *Service) StopBackgroundService()(Snapshot,error) { if !backgroundSupported(){return s.GetSnapshot(),errors.New("background tunnel management is unsupported")};if e:=stopBackground();e!=nil{return s.GetSnapshot(),e};return s.GetSnapshot(),nil }
func (s *Service) RemoveBackgroundService()(Snapshot,error) { if !backgroundSupported(){return s.GetSnapshot(),errors.New("background tunnel management is unsupported")};if e:=removeBackground();e!=nil{return s.GetSnapshot(),e};s.mu.Lock();s.backgroundNeedsRestart=false;s.mu.Unlock();return s.GetSnapshot(),nil }
func hasSystemctl()bool{_,e:=exec.LookPath("systemctl");return e==nil}
func systemctl(args ...string)(string,error){p,e:=exec.LookPath("systemctl");if e!=nil{return "",errors.New("systemctl not found")};ctx,c:=context.WithTimeout(context.Background(),8*time.Second);defer c();a:=append([]string{"--user"},args...);b,e:=exec.CommandContext(ctx,p,a...).CombinedOutput();if e!=nil{return string(b),fmt.Errorf("systemctl %s: %w: %s",strings.Join(args," "),e,strings.TrimSpace(string(b)))};return string(b),nil}
func unitActive(n string)bool{o,e:=systemctl("is-active",n);return e==nil&&strings.TrimSpace(o)=="active"}
func unitEnabled(n string)bool{o,e:=systemctl("is-enabled",n);return e==nil&&(strings.TrimSpace(o)=="enabled"||strings.TrimSpace(o)=="enabled-runtime")}
func fileExists(p string)bool{_,e:=os.Stat(p);return e==nil}
