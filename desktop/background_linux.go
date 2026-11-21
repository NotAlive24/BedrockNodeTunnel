//go:build linux
package main
import("context";"encoding/json";"errors";"os";"os/exec";"path/filepath";"strconv";"strings";"time")
type backgroundConfig struct{Binary string; Args []string}
func backgroundPaths()(string,string,error){d,e:=os.UserConfigDir();if e!=nil{return "","",e};return filepath.Join(d,"bedrocknode-tunnel","background.json"),filepath.Join(d,"systemd","user","bedrocknode-tunnel.service"),nil}
func configureBackground(binary string,args []string)error{
 state,unit,e:=backgroundPaths();if e!=nil{return e};if e=os.MkdirAll(filepath.Dir(state),0700);e!=nil{return e};if e=os.MkdirAll(filepath.Dir(unit),0700);e!=nil{return e}
 b,e:=json.Marshal(backgroundConfig{binary,args});if e!=nil{return e};if e=os.WriteFile(state,b,0600);e!=nil{return e}
 parts:=[]string{strconv.Quote(binary)};for _,a:=range args{parts=append(parts,strconv.Quote(a))}
 body:="[Unit]\nDescription=Nethernet Bedrock Node Tunnel\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nExecStart="+strings.Join(parts," ")+"\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
 tmp:=unit+".tmp";if e=os.WriteFile(tmp,[]byte(body),0600);e!=nil{return e};return os.Rename(tmp,unit)
}
func systemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	b, e := c.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("systemctl timed out")
	}
	if e != nil {
		return errors.New(strings.TrimSpace(string(b)) + ": " + e.Error())
	}
	return nil
}
func backgroundStatus() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "systemctl", "--user", "is-active", "--quiet", "bedrocknode-tunnel.service")
	err := c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return false, errors.New("systemctl status check timed out")
	}
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
func startBackground()error{if e:=systemctl("daemon-reload");e!=nil{return e};return systemctl("enable","--now","bedrocknode-tunnel.service")}
func stopBackground()error{return systemctl("stop","bedrocknode-tunnel.service")}
func restartBackground()error{return systemctl("restart","bedrocknode-tunnel.service")}
func removeBackground()error{_ = systemctl("disable","--now","bedrocknode-tunnel.service");_,u,e:=backgroundPaths();if e!=nil{return e};_ = os.Remove(u);return systemctl("daemon-reload")}
func backgroundSupported() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}
