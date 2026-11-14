//go:build linux

package backend

import ("errors";"fmt";"os";"path/filepath";"strings")

const autostartDesktopFile = "nethernet-desktop.desktop"
func autostartFilePath() (string,error) {
	configDir,err:=os.UserConfigDir();if err!=nil{return "",err}
	return filepath.Join(configDir,"autostart",autostartDesktopFile),nil
}
func quoteDesktopExecArg(value string) string {
	var b strings.Builder;b.WriteByte('"')
	for _,r:=range value { if r==92||r==34||r==36||r==96 { b.WriteByte(92) };b.WriteRune(r) }
	b.WriteByte('"');return b.String()
}
func renderAutostartEntry(executable string,startMinimized bool) string {
	args:=startupArguments(executable,startMinimized);for i:=range args {args[i]=quoteDesktopExecArg(args[i])}
	return "[Desktop Entry]\nType=Application\nVersion=1.0\nName=Nethernet Desktop\nComment=Bedrock Node Tunnel desktop companion\nExec="+strings.Join(args," ")+"\nTerminal=false\nStartupNotify=false\nX-GNOME-Autostart-enabled=true\n"
}
func configureAutostart(enabled,startMinimized bool) error {
	path,err:=autostartFilePath();if err!=nil{return err}
	if !enabled {err=os.Remove(path);if errors.Is(err,os.ErrNotExist){return nil};return err}
	executable,err:=os.Executable();if err!=nil{return fmt.Errorf("locate Nethernet Desktop executable: %w",err)}
	executable,err=filepath.Abs(executable);if err!=nil{return err}
	if err=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
	file,err:=os.CreateTemp(filepath.Dir(path),".nethernet-autostart-*.tmp");if err!=nil{return err}
	tmp:=file.Name();defer os.Remove(tmp)
	if err=file.Chmod(0600);err==nil {_,err=file.WriteString(renderAutostartEntry(executable,startMinimized))}
	if err==nil {err=file.Sync()};closeErr:=file.Close()
	if err!=nil{return err};if closeErr!=nil{return closeErr}
	return os.Rename(tmp,path)
}
