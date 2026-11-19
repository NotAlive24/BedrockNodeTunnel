package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed all:frontend
var frontend embed.FS

func main() {
	app := NewAppService()
	if err := runWebUI(app); err != nil {
		log.Fatal(err)
	}
}

func runWebUI(app *AppService) error {
	sub, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return fmt.Errorf("load embedded frontend: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start loopback listener: %w", err)
	}
	address := listener.Addr().String()
	server := &http.Server{
		Handler:           newWebHandler(app, address, sub, nil),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	url := "http://" + address + "/"
	log.Printf("Nethernet Desktop is available at %s", url)
	if err := openBrowserURL(url); err != nil {
		log.Printf("Could not open browser automatically: %v", err)
		log.Printf("Open this address manually: %s", url)
	}
	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			return fmt.Errorf("local web server stopped: %w", err)
		}
		return nil
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		_ = app.ServiceShutdown()
		ctxDone := make(chan struct{})
		go func() {
			_ = server.Close()
			close(ctxDone)
		}()
		select {
		case <-ctxDone:
		case <-time.After(3 * time.Second):
		}
		return nil
	}
}

func openBrowserURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return cmd.Start()
}
