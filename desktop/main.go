package main

import (
	"embed"
	"log"

	"github.com/NotAlive24/BedrockNodeTunnel/desktop/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend
var frontend embed.FS

func main() {
	backendService := backend.NewService()
	app := application.New(application.Options{
		Name:        "Nethernet Desktop",
		Description: "Desktop companion for Bedrock Node Tunnel",
		Services: []application.Service{
			application.NewService(backendService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend),
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "Nethernet Desktop",
		Width:            1180,
		Height:           780,
		MinWidth:         900,
		MinHeight:        620,
		BackgroundColour: application.NewRGB(13, 16, 22),
	})
	window.Center()
	window.Show()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
