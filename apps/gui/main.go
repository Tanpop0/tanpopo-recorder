package main

import (
	"embed"
	"runtime"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var iconDataWindows []byte

//go:embed build/appicon.png
var iconDataDefault []byte

func main() {
	app := NewApp()

	go func() {
		systray.Run(func() {
			onReady(app)
		}, func() {
			onExit(app)
		})
	}()

	err := wails.Run(&options.App{
		Title:     "twitcasting-recorder-gui",
		Width:     1280,
		Height:    760,
		MinWidth:  980,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "twitcasting-recorder-gui",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.ShowWindow()
			},
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}

	systray.Quit()
}

func onReady(app *App) {
	if runtime.GOOS == "windows" {
		if len(iconDataWindows) > 0 {
			systray.SetIcon(iconDataWindows)
		}
	} else if len(iconDataDefault) > 0 {
		systray.SetIcon(iconDataDefault)
	}

	systray.SetTitle("TwitCasting Recorder")
	systray.SetTooltip("TwitCasting Recorder 正在后台运行")

	mShow := systray.AddMenuItem("显示窗口", "恢复主窗口")
	mHide := systray.AddMenuItem("隐藏到后台", "隐藏主窗口，录制和监听继续运行")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出程序", "停止程序并退出")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				go app.ShowWindow()
			case <-mHide.ClickedCh:
				go app.HideWindow()
			case <-mQuit.ClickedCh:
				go app.Quit()
			}
		}
	}()
}

func onExit(app *App) {
	// Cleanup if needed.
}
