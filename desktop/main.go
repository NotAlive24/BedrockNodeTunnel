package main

import (
	"embed"
	"log"
	"os"
	"sync/atomic"

	"github.com/NotAlive24/BedrockNodeTunnel/desktop/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend
var frontend embed.FS

//go:embed assets/tray.svg
var trayIcon []byte

func main() {
	backendService := backend.NewService()
	app := application.New(application.Options{
		Name:        "Nethernet Desktop",
		Description: "Desktop companion for Bedrock Node Tunnel",
		Services: []application.Service{
			application.NewService(backendService),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend)},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "Nethernet Desktop", Width: 1180, Height: 780,
		MinWidth: 900, MinHeight: 620, BackgroundColour: application.NewRGB(13, 16, 22),
	})
	window.Center()

	var quitting atomic.Bool
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetLabel("Nethernet Desktop")
	menu := app.NewMenu()
	menu.Add("Open Nethernet Desktop").OnClick(func(ctx *application.Context) {
		window.Show()
		window.Focus()
	})
	menu.AddSeparator()
	menu.Add("Start tunnel").OnClick(func(ctx *application.Context) {
		if _, err := backendService.StartClient(); err != nil {
			log.Printf("tray start: %v", err)
		}
	})
	menu.Add("Stop tunnel").OnClick(func(ctx *application.Context) {
		if _, err := backendService.StopClient(); err != nil {
			log.Printf("tray stop: %v", err)
		}
	})
	menu.AddSeparator()
	menu.Add("Quit Nethernet Desktop").OnClick(func(ctx *application.Context) {
		quitting.Store(true)
		app.Quit()
	})
	tray.SetMenu(menu)
	tray.OnClick(func() {
		window.Show()
		window.Focus()
	})

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !quitting.Load() && backendService.GetSettings().CloseToTray {
			e.Cancel()
			window.Hide()
		}
	})

	app.OnShutdown(func() {
		if err := backendService.ServiceShutdown(); err != nil {
			log.Printf("shutdown: %v", err)
		}
	})

	startHidden := false
	for _, arg := range os.Args[1:] { if arg == "--background" { startHidden = true; break } }
	if !startHidden { window.Show() }
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
