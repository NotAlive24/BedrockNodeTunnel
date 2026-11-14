//go:build windows

package backend

import ("fmt";"os";"os/exec";"path/filepath";"strings")

const windowsRunKey = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
const windowsRunValue = "NethernetDesktop"
func windowsRegistryTool()(string,error) {
	if path,err:=exec.LookPath("reg.exe");err==nil{return path,nil}
	root:=os.Getenv("SystemRoot");if root!="" {path:=filepath.Join(root,"System32","reg.exe");if _,err:=os.Stat(path);err==nil{return path,nil}}
	return "",fmt.Errorf("reg.exe was not found; cannot configure Windows autostart")
}
func windowsStartupCommand(executable string,startMinimized bool) string {
	command:="\""+executable+"\"";if startMinimized {command+=" --background"};return command
}
func configureAutostart(enabled,startMinimized bool) error {
	reg,err:=windowsRegistryTool();if err!=nil{return err}
	if !enabled {
		output,err:=exec.Command(reg,"delete",windowsRunKey,"/v",windowsRunValue,"/f").CombinedOutput()
		if err!=nil {message:=strings.ToLower(string(output));if strings.Contains(message,"unable to find")||strings.Contains(message,"cannot find"){return nil};return fmt.Errorf("remove Windows autostart entry: %w: %s",err,strings.TrimSpace(string(output)))}
		return nil
	}
	executable,err:=os.Executable();if err!=nil{return fmt.Errorf("locate Nethernet Desktop executable: %w",err)}
	executable,err=filepath.Abs(executable);if err!=nil{return err}
	output,err:=exec.Command(reg,"add",windowsRunKey,"/v",windowsRunValue,"/t","REG_SZ","/d",windowsStartupCommand(executable,startMinimized),"/f").CombinedOutput()
	if err!=nil{return fmt.Errorf("configure Windows autostart: %w: %s",err,strings.TrimSpace(string(output)))}
	return nil
}
