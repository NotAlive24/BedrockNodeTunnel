//go:build !linux && !windows
package main
import "errors"
func configureBackground(string,[]string)error{return errors.New("background mode is not supported on this operating system")}
func backgroundStatus()(bool,error){return false,nil}
func startBackground()error{return errors.New("background mode is not supported on this operating system")}
func stopBackground()error{return errors.New("background mode is not supported on this operating system")}
func restartBackground()error{return errors.New("background mode is not supported on this operating system")}
func removeBackground()error{return errors.New("background mode is not supported on this operating system")}
func backgroundSupported()bool{return false}
