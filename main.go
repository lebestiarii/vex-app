package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strings"

	"vex/internal/models"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

func runValidateCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: vex validate <module.json> [more.json...]")
		return 1
	}
	exitCode := 0
	for _, path := range args {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "FAIL %s: %v\n", path, err)
			exitCode = 1
			continue
		}
		mod, res := models.ValidateModuleJSON(raw)
		if !res.Valid {
			fmt.Fprintf(stderr, "FAIL %s (%d error(s)):\n", path, len(res.Errors))
			for _, fe := range res.Errors {
				fmt.Fprintf(stderr, "  - %s\n", fe.String())
			}
			exitCode = 1
			continue
		}
		fmt.Fprintf(stdout, "OK %s (module=%s cards=%d patterns=%d)\n", path, mod.ID, len(mod.Cards), len(mod.Patterns))
	}
	return exitCode
}

func main() {
	// 0. CLI subcommand: `vex validate <path.json> [...]`
	if len(os.Args) >= 2 && os.Args[1] == "validate" {
		os.Exit(runValidateCLI(os.Args[2:], os.Stdout, os.Stderr))
	}

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

	initialPrefs := normalizeUserPrefs(cfg.Prefs)

	launcherWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            info.Name,
		Width:            initialPrefs.UISize,
		Height:           680,
		MinWidth:         420,
		MinHeight:        120,
		Frameless:        true,
		AlwaysOnTop:      initialPrefs.StayAwake,
		Hidden:           initialPrefs.OnboardingCompleted,
		HideOnFocusLost:  false,
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

	launcherWin.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) {
		if !backend.AppService.GetStayAwake() {
			application.InvokeAsync(func() {
				launcherWin.Hide()
			})
		}
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
			launcherWin.EmitEvent("vex:focus")
		})
	}

	backend.AppService.SetHideHandler(func() {
		application.InvokeAsync(func() {
			launcherWin.Hide()
		})
	})

	backend.AppService.SetResizeHandler(func(width, height int) {
		clampedH := height
		if clampedH < 120 {
			clampedH = 120
		}
		if clampedH > 720 {
			clampedH = 720
		}
		application.InvokeAsync(func() {
			oldW, oldH := launcherWin.Size()
			clampedW := width
			if clampedW <= 0 {
				clampedW = oldW
			}
			if clampedW < 420 {
				clampedW = 480
			}
			if clampedW > 660 {
				clampedW = 660
			}
			if clampedW == oldW && clampedH == oldH {
				return
			}
			relX, relY := launcherWin.RelativePosition()
			launcherWin.SetSize(clampedW, clampedH)
			if oldW > 0 && clampedW != oldW {
				newRelX := relX - (clampedW-oldW)/2
				screen, _ := launcherWin.GetScreen()
				if screen == nil {
					screen = app.Screen.GetPrimary()
				}
				if screen != nil {
					wa := WorkAreaFromScreen(screen)
					maxX := wa.Width - clampedW - defaultWindowMargin
					if newRelX < defaultWindowMargin {
						newRelX = defaultWindowMargin
					}
					if maxX > defaultWindowMargin && newRelX > maxX {
						newRelX = maxX
					}
				}
				launcherWin.SetRelativePosition(newRelX, relY)
			}
		})
	})

	backend.AppService.SetStayAwakeHandler(func(enabled bool) {
		application.InvokeAsync(func() {
			launcherWin.SetAlwaysOnTop(enabled)
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
