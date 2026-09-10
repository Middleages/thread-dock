//go:build windows

package main

import (
	"embed"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"thread-dock/internal/runner"
)

// The placeholder under frontend/dist keeps clean-checkout Go compilation
// deterministic; Wails replaces it with the Vite build for production.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	env := environmentMap(os.Environ())
	process := runner.OSRunner{}
	github := NewGitHubMonitor(env, process, 15*time.Second)
	herdr := NewHerdrMonitor(env, process, 15*time.Second)
	app := NewApp(NewCombinedMonitor(github, herdr))
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

func environmentMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, value := range values {
		for index := 0; index < len(value); index++ {
			if value[index] == '=' {
				env[value[:index]] = value[index+1:]
				break
			}
		}
	}
	return env
}
