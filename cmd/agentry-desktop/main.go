//go:build desktop

package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"

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

	a := &window{zoom: 1, log: redactedLog{w: os.Stderr, token: token}}
	agentry, failed := launch.Ensure(o)
	if failed == nil {
		defer agentry.Stop() // only a server this app started
		a.page = launch.PageURL(agentry.Client)
		a.origin = originOf(a.page)
	}
	a.start = startPage(a.page, failed, token())
	if err := wails.Run(a.options()); err != nil {
		a.log.Error(err.Error())
		return 1
	}
	return 0
}

// window is the app's one window and what it shows.
type window struct {
	ctx          context.Context
	page, origin string // Agentry's signed-in page and its origin; empty when it didn't start
	start        []byte // the page the window opens on
	log          redactedLog
	listen       sync.Once // for linkEvent, from the first page load on

	mu   sync.Mutex
	zoom float64
}

// options are the app's: closing the window quits it on every system (see
// docs/decisions.md), and a second launch shows this window instead.
func (w *window) options() *options.App {
	return &options.App{
		Title:     "Agentry",
		Width:     1280,
		Height:    860,
		MinWidth:  480,
		MinHeight: 360,
		AssetServer: &assetserver.Options{Handler: http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.Header().Set("Cache-Control", "no-store")
			rw.Write(w.start)
		})},
		Menu:                     w.menu(),
		OnStartup:                func(ctx context.Context) { w.ctx = ctx },
		OnDomReady:               w.loaded,
		EnableDefaultContextMenu: true,
		BindingsAllowedOrigins:   w.origin, // so the page's links reach linkEvent
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

// loaded runs after every page load: it routes links and keeps the zoom.
func (w *window) loaded(ctx context.Context) {
	if w.origin == "" {
		return
	}
	w.listen.Do(func() {
		follow := links{
			origin:  w.origin,
			browser: func(link string) { runtime.BrowserOpenURL(ctx, link) },
			window:  func(link string) { runtime.WindowExecJS(ctx, goScript(link)) },
		}
		runtime.EventsOn(ctx, linkEvent, follow.follow)
	})
	runtime.WindowExecJS(ctx, pageScript(w.origin, w.zoomLevel()))
}

func (w *window) menu() *menu.Menu {
	m := menu.NewMenu()
	if goruntime.GOOS == "darwin" {
		m.Append(menu.AppMenu()) // has Quit, ⌘Q
		m.Append(menu.EditMenu())
	}
	file := m.AddSubmenu("File")
	file.AddText("Open in browser", keys.CmdOrCtrl("b"), func(*menu.CallbackData) {
		if w.page != "" {
			runtime.BrowserOpenURL(w.ctx, w.page)
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
