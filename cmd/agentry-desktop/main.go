//go:build desktop

package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/launch"
)

//go:embed build/appicon.png
var icon []byte

func main() { os.Exit(run()) }

func run() int {
	engines.DropParentSession() // so the sessions a started server runs aren't taken for a parent's
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	o := launchOptions(home, exe, goruntime.GOOS, nil)
	token := func() string { return tokenIn(o.DataDir) }
	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "agentry-desktop:", err)
		return 1
	}
	serveLog, err := os.OpenFile(filepath.Join(o.DataDir, "serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentry-desktop:", err)
		return 1
	}
	defer serveLog.Close()
	output := &launch.Redactor{W: serveLog, Token: token}
	defer output.Close()
	o.Output = output

	// Production Wails doesn't handle these, so it would end without
	// stopping the server; quit the window instead.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	a := &window{agentry: &agentry{launch: o, token: token}, zoom: 1, log: redactedLog{w: os.Stderr, token: token}, signals: signals}
	defer a.stop()
	var running string
	if c, ok := launch.Running(o.DataDir); ok {
		running = originOf(launch.PageURL(c))
	}
	if err := wails.Run(a.options(running)); err != nil {
		a.log.Error(err.Error())
		return 1
	}
	return 0
}

// window is the app's one window and what it shows.
type window struct {
	*agentry
	ctx     context.Context
	log     redactedLog
	signals <-chan os.Signal
	listen  sync.Once // for linkEvent, from the first page load on

	mu   sync.Mutex
	zoom float64
}

// options are the app's: closing the window quits it on every system (see
// docs/decisions.md), and a second launch shows this window instead.
// running is the origin of the server answering now, if any.
func (w *window) options(running string) *options.App {
	return &options.App{
		Title:                    "Agentry",
		Width:                    1280,
		Height:                   860,
		MinWidth:                 480,
		MinHeight:                360,
		AssetServer:              &assetserver.Options{Handler: http.HandlerFunc(w.serve)},
		Menu:                     w.menu(),
		OnStartup:                w.startup,
		OnDomReady:               w.loaded,
		OnShutdown:               func(context.Context) { w.stop() },
		EnableDefaultContextMenu: true,
		BindingsAllowedOrigins:   bindingsOrigins(running), // so the page's links reach linkEvent
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "agentry-desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				runtime.WindowUnminimise(w.ctx)
				runtime.WindowShow(w.ctx)
			},
		},
		Logger:             w.log,
		LogLevel:           logger.WARNING,
		LogLevelProduction: logger.ERROR,
		Linux:              &linux.Options{Icon: icon, ProgramName: "Agentry"},
		Mac:                &mac.Options{About: &mac.AboutInfo{Title: "Agentry", Icon: icon}},
	}
}

// startup keeps the window's context, and quits on SIGINT or SIGTERM.
func (w *window) startup(ctx context.Context) {
	w.ctx = ctx
	go func() {
		<-w.signals
		runtime.Quit(ctx)
	}()
}

// loaded runs after every page load: it routes links and keeps the zoom.
func (w *window) loaded(ctx context.Context) {
	_, origin := w.signedIn()
	if origin == "" {
		return
	}
	w.listen.Do(func() {
		follow := links{
			origin:  origin,
			browser: func(link string) { runtime.BrowserOpenURL(ctx, link) },
			window:  func(link string) { runtime.WindowExecJS(ctx, goScript(link)) },
		}
		runtime.EventsOn(ctx, linkEvent, follow.follow)
	})
	runtime.WindowExecJS(ctx, pageScript(startURL(goruntime.GOOS), origin, w.zoomLevel()))
}

func (w *window) menu() *menu.Menu {
	m := menu.NewMenu()
	if goruntime.GOOS == "darwin" {
		m.Append(menu.AppMenu()) // has Quit, ⌘Q
		m.Append(menu.EditMenu())
	}
	file := m.AddSubmenu("File")
	file.AddText("Open in browser", keys.CmdOrCtrl("b"), func(*menu.CallbackData) {
		if page, _ := w.signedIn(); page != "" {
			runtime.BrowserOpenURL(w.ctx, page)
		}
	})
	if goruntime.GOOS != "darwin" {
		file.AddSeparator()
		file.AddText("Quit", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { runtime.Quit(w.ctx) })
	}
	view := m.AddSubmenu("View")
	view.AddText("Reload", keys.CmdOrCtrl("r"), func(*menu.CallbackData) { runtime.WindowExecJS(w.ctx, reloadScript) })
	view.AddSeparator()
	view.AddText("Zoom in", keys.CmdOrCtrl("="), func(*menu.CallbackData) { w.setZoom(zoomIn) })
	view.AddText("Zoom out", keys.CmdOrCtrl("-"), func(*menu.CallbackData) { w.setZoom(zoomOut) })
	view.AddText("Actual size", keys.CmdOrCtrl("0"), func(*menu.CallbackData) { w.setZoom(func(float64) float64 { return 1 }) })
	return m
}

func (w *window) zoomLevel() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.zoom
}

// setZoom changes the zoom, kept for the next page load too.
func (w *window) setZoom(next func(float64) float64) {
	w.mu.Lock()
	w.zoom = next(w.zoom)
	zoom := w.zoom
	w.mu.Unlock()
	runtime.WindowExecJS(w.ctx, zoomScript(zoom))
}
