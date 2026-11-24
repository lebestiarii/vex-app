package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

func main() {
	mcpStdio := flag.Bool("mcp-stdio", false, "Run headless MCP JSON-RPC server over stdio (for Claude Desktop, Cursor, Copilot CLI, VS Code)")
	dataDir := flag.String("data-dir", "", "Override base data directory (default ~/.vex)")
	mcpAddrFlag := flag.String("mcp-addr", "", "Override HTTP MCP server address (default 127.0.0.1:17380; pass 'off' to disable)")
	hotkeyFlag := flag.String("hotkey", "", "Override global shortcut accelerator (default Ctrl+Shift+B)")
	flag.Parse()

	info := currentAppInfo()

	// 1. Headless --mcp-stdio mode: no GUI, no stdout noise, pure JSON-RPC on stdin/stdout.
	if *mcpStdio {
		backend, err := InitBackend(*dataDir, false)
		if err != nil {
			log.Fatalf("init backend: %v", err)
		}
		defer backend.Close()

		if err := backend.MCPHub.ServeStdio(); err != nil {
			log.Fatalf("mcp stdio server: %v", err)
		}
		return
	}

	// 2. Full Desktop Tray + Global Shortcut + Streamable HTTP MCP Hub mode.
	backend, err := InitBackend(*dataDir, true)
	if err != nil {
		log.Fatalf("init backend: %v", err)
	}

	cfg := backend.Config()
	mcpAddr := cfg.MCPAddr
	if strings.TrimSpace(*mcpAddrFlag) != "" {
		mcpAddr = strings.TrimSpace(*mcpAddrFlag)
	}
	hotkey := cfg.Hotkey
	if strings.TrimSpace(*hotkeyFlag) != "" {
		hotkey = strings.TrimSpace(*hotkeyFlag)
	}

	if !strings.EqualFold(mcpAddr, "off") && mcpAddr != "" {
		go func() {
			if err := backend.MCPHub.StartHTTP(mcpAddr); err != nil {
				log.Printf("MCP HTTP server stopped: %v", err)
			}
		}()
	}

	distFS, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		log.Fatalf("embed frontend/dist: %v", err)
	}

	app := application.New(application.Options{
		Name:        info.Name,
		Description: info.Name + " — Local-first desktop search launcher and knowledge hub",
		Services: []application.Service{
			application.NewService(backend.AppService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(distFS),
		},
		Mac: application.MacOptions{
			ActivationPolicy:                                application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	launcherWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            info.Name,
		Width:            480,
		Height:           680,
		MinWidth:         420,
		MinHeight:        480,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           cfg.Prefs.OnboardingCompleted,
		HideOnFocusLost:  true,
		InitialPosition:  application.WindowCentered,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			DisableShadow: true,
		},
		Windows: application.WindowsWindow{
			DisableFramelessWindowDecorations: true,
			HiddenOnTaskbar:                   true,
		},
	})

	// System Tray integration
	systray := app.SystemTray.New()
	if icon := trayIconBytes(); len(icon) > 0 {
		systray.SetIcon(icon)
	} else {
		systray.SetLabel(info.Name)
	}
	systray.SetTooltip(info.Name)

	toggleLauncher := func(trigger SpawnTrigger) {
		application.InvokeAsync(func() {
			if launcherWin.IsVisible() {
				launcherWin.Hide()
				return
			}

			winW, winH := launcherWin.Size()
			if winW <= 0 {
				winW = 780
			}
			if winH <= 0 {
				winH = 540
			}

			anchor := AnchorDown
			if trigger == TriggerTray {
				if err := systray.PositionWindow(launcherWin, defaultWindowMargin); err == nil {
					screen, _ := launcherWin.GetScreen()
					if screen == nil {
						screen = app.Screen.GetPrimary()
					}
					_, relY := launcherWin.RelativePosition()
					anchor = DetectAnchorFromWindowPosition(WorkAreaFromScreen(screen), relY, winH)
				} else {
					screen, _ := launcherWin.GetScreen()
					if screen == nil {
						screen = app.Screen.GetPrimary()
					}
					p := ComputeWindowPlacementForOS(runtime.GOOS, WorkAreaFromScreen(screen), winW, winH, TriggerTray, nil, defaultWindowMargin)
					launcherWin.SetRelativePosition(p.RelX, p.RelY)
					anchor = p.Anchor
				}
			} else {
				screen, _ := launcherWin.GetScreen()
				if screen == nil {
					screen = app.Screen.GetPrimary()
				}
				p := ComputeWindowPlacementForOS(runtime.GOOS, WorkAreaFromScreen(screen), winW, winH, TriggerHotkey, nil, defaultWindowMargin)
				launcherWin.SetRelativePosition(p.RelX, p.RelY)
				anchor = p.Anchor
			}

			backend.AppService.SetAnchorDirection(anchor)
			launcherWin.EmitEvent("vex:anchor", string(anchor))
			launcherWin.Show()
			launcherWin.Focus()
		})
	}

	backend.AppService.SetHideHandler(func() {
		application.InvokeAsync(func() {
			launcherWin.Hide()
		})
	})

	systray.OnClick(func() {
		toggleLauncher(TriggerTray)
	})

	trayMenu := app.NewMenu()
	toggleMenuItem := trayMenu.Add("Toggle " + info.Name + " (" + hotkey + ")").OnClick(func(_ *application.Context) {
		toggleLauncher(TriggerTray)
	})
	launchAtLoginItem := trayMenu.AddCheckbox("Launch at Login", backend.AppService.GetLaunchAtLogin())
	launchAtLoginItem.OnClick(func(_ *application.Context) {
		next := !backend.AppService.GetLaunchAtLogin()
		applied, err := backend.AppService.SetLaunchAtLogin(next)
		if err != nil {
			app.Logger.Error("Failed to toggle launch at login", "enabled", next, "error", err)
		}
		application.InvokeAsync(func() {
			launchAtLoginItem.SetChecked(applied)
		})
	})
	backend.AppService.SetLaunchAtLoginChangeHandler(func(enabled bool) {
		application.InvokeAsync(func() {
			launchAtLoginItem.SetChecked(enabled)
		})
	})
	if !strings.EqualFold(mcpAddr, "off") && mcpAddr != "" {
		trayMenu.Add("MCP Server: http://" + mcpAddr + "/mcp").SetEnabled(false)
	}
	trayMenu.AddSeparator()
	trayMenu.Add("Quit " + info.Name).OnClick(func(_ *application.Context) {
		app.Quit()
	})
	systray.SetMenu(trayMenu)

	// Global Hotkey integration + live rebinding from Settings UI
	if err := app.GlobalShortcut.Register(hotkey, func() { toggleLauncher(TriggerHotkey) }); err != nil {
		app.Logger.Error("Failed to register global shortcut", "hotkey", hotkey, "error", err)
	}
	backend.AppService.SetHotkeyChangeHandler(func(oldKey, newKey string) error {
		if oldKey != "" {
			_ = app.GlobalShortcut.Unregister(oldKey)
		}
		if err := app.GlobalShortcut.Register(newKey, func() { toggleLauncher(TriggerHotkey) }); err != nil {
			// Roll back to previous hotkey if OS rejects the new combination
			if oldKey != "" {
				_ = app.GlobalShortcut.Register(oldKey, func() { toggleLauncher(TriggerHotkey) })
			}
			return err
		}
		application.InvokeAsync(func() {
			toggleMenuItem.SetLabel("Toggle " + info.Name + " (" + newKey + ")")
		})
		return nil
	})

	app.OnShutdown(func() {
		backend.Close()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
