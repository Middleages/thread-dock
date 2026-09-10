//go:build windows

package main

import (
	"embed"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"thread-dock/internal/monitorcli"
	"thread-dock/internal/runner"
)

// The placeholder under frontend/dist keeps clean-checkout Go compilation
// deterministic; Wails replaces it with the Vite build for production.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp(monitorcli.New(runner.OSRunner{}, 5*time.Second))
	if err := wails.Run(&options.App{
		Title: "ThreadDock Monitor",
		Width: 1280, Height: 800,
		MinWidth: 680, MinHeight: 520,
		Bind:        []interface{}{app},
		AssetServer: &assetserver.Options{Assets: assets},
		Mac:         &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
	}); err != nil {
		panic(err)
	}
}
