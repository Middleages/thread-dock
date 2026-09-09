//go:build windows

package main

import (
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"thread-dock/internal/monitorcli"
	"thread-dock/internal/runner"
)

func main() {
	app := NewApp(monitorcli.New(runner.OSRunner{}, 5*time.Second))
	if err := wails.Run(&options.App{
		Title: "ThreadDock Monitor",
		Width: 1280, Height: 800,
		MinWidth: 680, MinHeight: 520,
		Bind: []interface{}{app},
		Mac:  &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
	}); err != nil {
		panic(err)
	}
}
