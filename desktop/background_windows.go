//go:build windows
package main
import("encoding/json";"errors";"os";"os/exec";"path/filepath";"strings")
const taskName="NethernetBedrockNodeTunnel"
type backgroundConfig struct{Binary string;Args []string}
func backgroundPath()(string,error){d,e:=os.UserConfigDir();if e!=nil{return "",e};return filepath.Join(d,"bedrocknode-tunnel","background.json"),nil}
func configureBackground(binary string,args []string)error{p,e:=backgroundPath();if e!=nil{return e};if e=os.MkdirAll(filepath.Dir(p),0700);e!=nil{return e};b,e:=json.Marshal(backgroundConfig{binary,args});if e!=nil{return e};return os.WriteFile(p,b,0600)}
func taskCommand()(string,error){p,e:=backgroundPath();if e!=nil{return "",e};b,e:=os.ReadFile(p);if e!=nil{return "",e};var c backgroundConfig;if e=json.Unmarshal(b,&c);e!=nil{return "",e};q:=func(s string)string{if !strings.ContainsAny(s," \t\""){return s};return "\""+strings.ReplaceAll(s,"\"","\\\"")+"\""};a:=[]string{q(c.Binary)};for _,v:=range c.Args{a=append(a,q(v))};return strings.Join(a," "),nil}
func task(args ...string)error{b,e:=exec.Command("schtasks.exe",args...).CombinedOutput();if e!=nil{return errors.New(strings.TrimSpace(string(b))+": "+e.Error())};return nil}
func backgroundStatus()(bool,error){return exec.Command("schtasks.exe","/Query","/TN",taskName).Run()==nil,nil}
func startBackground()error{c,e:=taskCommand();if e!=nil{return e};if e=task("/Create","/SC","ONLOGON","/TN",taskName,"/TR",c,"/F");e!=nil{return e};return task("/Run","/TN",taskName)}
func stopBackground()error{return task("/End","/TN",taskName)}
func restartBackground()error{_ = stopBackground();return task("/Run","/TN",taskName)}
func removeBackground()error{_ = stopBackground();if e:=task("/Delete","/TN",taskName,"/F");e!=nil{return e};p,e:=backgroundPath();if e==nil{_ = os.Remove(p)};return nil}
func backgroundSupported()bool{return true}
