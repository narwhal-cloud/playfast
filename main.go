package main

import (
	"embed"
	"log"
	"net"
	"os"
	"playfast/internal/path"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var logo []byte

var Version = "1.0.0"

func main() {
	dial, err := net.Dial("tcp", "127.0.0.1:54712")
	if err == nil {
		_, _ = dial.Write([]byte("SHOW_WINDOW"))
		_ = dial.Close()
		return
	}
	appLog := path.Path() + "/app.log"
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	open, err := os.Create(appLog)
	if err == nil {
		log.SetOutput(open)
	}
	// Create an instance of the app structure
	app := NewApp()
	// Create application with options
	err = wails.Run(&options.App{
		Title:            "PlayFast",
		Width:            576,
		Height:           484,
		BackgroundColour: &options.RGBA{R: 75, G: 0, B: 130, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:         app.startup,
		DisableResize:     true,
		HideWindowOnClose: true,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
