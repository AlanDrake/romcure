package main

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"romcure/frontend"
	"romcure/internal/app"
	"romcure/internal/applog"
)

func main() {
	dataDir, err := applog.AppDataDir()
	if err != nil {
		log.Fatalf("resolve app data dir: %v", err)
	}
	logger, closeLog, err := applog.NewInDir(dataDir)
	if err != nil {
		log.Fatalf("open log file: %v", err)
	}
	defer closeLog()
	logger.Info("starting", "app_data_dir", dataDir)

	// The application is created before the library is opened, so a second
	// launch (an impatient double-click while the first one upgrades the
	// database) exits at once and brings the running window forward, instead
	// of waiting invisibly on the same database.
	var win *application.WebviewWindow
	svc := &Service{}
	wapp := application.New(application.Options{
		Name:        "romcure",
		Description: "ROM Library Manager (placeholder product name)",
		Services: []application.Service{
			application.NewService(svc),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "romcure.app",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if win != nil {
					win.UnMinimise()
					win.Show()
					win.Focus()
				}
			},
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	opened := time.Now()
	core, err := app.Open(dataDir, findTools(), logger)
	if err != nil {
		log.Fatalf("open library: %v", err)
	}
	defer core.Close()
	logger.Info("library open", "took", time.Since(opened).Round(time.Millisecond).String())
	if core.Warning != nil {
		logger.Warn("startup warning", "error", core.Warning)
	}
	svc.core = core

	win = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "RomCure",
		Width:  1100,
		Height: 740,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// Closing while a sync, update or conversion is running would throw its work away
	// half done, so ask first. Confirming stops the job cleanly, then closes.
	var closing atomic.Bool
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		job := svc.runningJob()
		if job == "" || closing.Load() {
			return
		}
		e.Cancel()
		what := map[string]string{"sync": "A sync", "apply": "An update of your outputs", "dats": "A database update", "chd": "A CHD check"}[job]
		if what == "" {
			what = "A task"
		}
		d := wapp.Dialog.Question().SetTitle("RomCure is still working").
			SetMessage(what + " is still running. If you close now it is stopped; what was finished is kept, and the next sync picks up where this one left off.")
		stay := d.AddButton("Keep working")
		quit := d.AddButton("Stop and close")
		quit.OnClick(func() {
			closing.Store(true)
			go func() {
				svc.stopAndWait(15 * time.Second)
				win.Close()
			}()
		})
		d.SetDefaultButton(stay).SetCancelButton(stay)
		d.Show()
	})

	if err := wapp.Run(); err != nil {
		logger.Error("app exited with error", "error", err)
		log.Fatal(err)
	}
	logger.Info("exiting")
}
