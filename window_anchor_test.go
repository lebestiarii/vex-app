package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestComputeWindowPlacement_CenteredHotkey(t *testing.T) {
	wa := WorkAreaRect{X: 0, Y: 25, Width: 1440, Height: 875, ScaleFactor: 2.0}
	p := ComputeWindowPlacementForOS("darwin", wa, 780, 540, TriggerHotkey, nil, 12)
	if p.Anchor != AnchorDown {
		t.Fatalf("expected AnchorDown for centered hotkey, got %q", p.Anchor)
	}
	if p.RelX != 330 || p.RelY != 167 {
		t.Fatalf("expected centered rel position (330,167), got (%d,%d)", p.RelX, p.RelY)
	}
	if p.AbsX != 330 || p.AbsY != 192 {
		t.Fatalf("expected abs position (330,192), got (%d,%d)", p.AbsX, p.AbsY)
	}
}

func TestComputeWindowPlacement_WindowsBottomRightTrayFlip(t *testing.T) {
	// 1920x1080 @ 150% DPI -> logical 1280x720 with 40px taskbar -> WorkArea 1280x680
	wa := WorkAreaRect{X: 0, Y: 0, Width: 1280, Height: 680, ScaleFactor: 1.5}
	p := ComputeWindowPlacementForOS("windows", wa, 780, 540, TriggerTray, nil, 12)
	if p.Anchor != AnchorUp {
		t.Fatalf("expected AnchorUp for Windows tray spawn, got %q", p.Anchor)
	}
	if p.RelX != 1280-780-12 {
		t.Fatalf("expected right-aligned X=%d, got %d", 1280-780-12, p.RelX)
	}
	if p.RelY != 680-540-12 {
		t.Fatalf("expected bottom-anchored Y=%d, got %d", 680-540-12, p.RelY)
	}
	if p.RelY+540 > wa.Height {
		t.Fatalf("window bottom %d clips work area height %d", p.RelY+540, wa.Height)
	}
}

func TestComputeWindowPlacement_BottomBandTargetFlipsUpMultiMonitor(t *testing.T) {
	// Secondary monitor placed to the left of primary with negative X coordinates
	wa := WorkAreaRect{X: -1920, Y: 0, Width: 1920, Height: 1040, ScaleFactor: 1.0}
	// Spawn point at 85% down the secondary screen (within bottom 28% band)
	pt := &application.Point{X: -200, Y: 920}
	p := ComputeWindowPlacementForOS("windows", wa, 780, 540, TriggerTray, pt, 12)
	if p.Anchor != AnchorUp {
		t.Fatalf("expected AnchorUp in bottom 28%% band, got %q", p.Anchor)
	}
	if p.RelX < 12 || p.RelX+780 > wa.Width-12 {
		t.Fatalf("RelX %d out of clamped bounds for width %d", p.RelX, wa.Width)
	}
	if p.AbsX != wa.X+p.RelX {
		t.Fatalf("expected AbsX=%d, got %d", wa.X+p.RelX, p.AbsX)
	}
	if p.RelY < 12 || p.RelY+540 > wa.Height-12 {
		t.Fatalf("RelY %d out of clamped bounds for height %d", p.RelY, wa.Height)
	}
}

func TestComputeWindowPlacement_MacOSTopMenuBarTrayStaysDown(t *testing.T) {
	wa := WorkAreaRect{X: 0, Y: 38, Width: 1512, Height: 944, ScaleFactor: 2.0}
	p := ComputeWindowPlacementForOS("darwin", wa, 780, 540, TriggerTray, nil, 12)
	if p.Anchor != AnchorDown {
		t.Fatalf("expected AnchorDown for macOS top tray, got %q", p.Anchor)
	}
	if p.RelY != 12 {
		t.Fatalf("expected top-aligned RelY=12, got %d", p.RelY)
	}
}

func TestDetectAnchorFromWindowPosition(t *testing.T) {
	wa := WorkAreaRect{X: 0, Y: 0, Width: 1920, Height: 1040, ScaleFactor: 1.0}

	// Flush above Windows taskbar -> AnchorUp
	if got := DetectAnchorFromWindowPosition(wa, 1040-540-10, 540); got != AnchorUp {
		t.Fatalf("expected AnchorUp when docked above taskbar, got %q", got)
	}

	// Flush below macOS menu bar -> AnchorDown
	if got := DetectAnchorFromWindowPosition(wa, 10, 540); got != AnchorDown {
		t.Fatalf("expected AnchorDown when docked below menu bar, got %q", got)
	}

	// Centered on screen -> AnchorDown
	if got := DetectAnchorFromWindowPosition(wa, 250, 540); got != AnchorDown {
		t.Fatalf("expected AnchorDown when centered, got %q", got)
	}
}

func TestPhysicalToLogicalWorkArea_HiDPI(t *testing.T) {
	// 4K display (3840x2160) at 200% scaling with 80px physical taskbar
	phys := WorkAreaRect{X: 3840, Y: 0, Width: 3840, Height: 2080, ScaleFactor: 2.0}
	got := PhysicalToLogicalWorkArea(phys)
	if got.X != 1920 || got.Y != 0 || got.Width != 1920 || got.Height != 1040 {
		t.Fatalf("unexpected 200%% logical work area: %+v", got)
	}

	// 150% Windows laptop display (2880x1800) with 60px physical taskbar
	phys150 := WorkAreaRect{X: 0, Y: 0, Width: 2880, Height: 1740, ScaleFactor: 1.5}
	got150 := PhysicalToLogicalWorkArea(phys150)
	if got150.Width != 1920 || got150.Height != 1160 {
		t.Fatalf("unexpected 150%% logical work area: %+v", got150)
	}
}
