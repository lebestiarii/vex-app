package main

import (
	"math"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AnchorDirection controls whether the launcher expands downward from a top search bar
// ("down") or upward from a bottom-anchored search bar ("up").
type AnchorDirection string

const (
	AnchorDown AnchorDirection = "down"
	AnchorUp   AnchorDirection = "up"
)

// SpawnTrigger identifies how the user invoked the launcher window.
type SpawnTrigger string

const (
	TriggerHotkey SpawnTrigger = "hotkey"
	TriggerTray   SpawnTrigger = "tray"
)

const (
	defaultWindowMargin = 12
	// bottomFlipThresholdRatio triggers upward anchoring when the spawn point
	// falls within the bottom 28% of the active display's work area (25-30% band).
	bottomFlipThresholdRatio = 0.72
)

// WorkAreaRect represents a display's usable work area in logical DIP coordinates
// (excluding macOS menu bar / Dock or Windows taskbar), plus its DPI scale factor.
type WorkAreaRect struct {
	X           int
	Y           int
	Width       int
	Height      int
	ScaleFactor float32
}

// WindowPlacement describes the target position and anchor direction for the launcher.
type WindowPlacement struct {
	RelX        int // X relative to WorkArea origin (for SetRelativePosition)
	RelY        int // Y relative to WorkArea origin (for SetRelativePosition)
	AbsX        int // Absolute logical DIP X
	AbsY        int // Absolute logical DIP Y
	Anchor      AnchorDirection
	ScaleFactor float32
}

// NormalizeAnchor validates a raw anchor string, defaulting to AnchorDown.
func NormalizeAnchor(raw string) AnchorDirection {
	if strings.EqualFold(strings.TrimSpace(raw), string(AnchorUp)) {
		return AnchorUp
	}
	return AnchorDown
}

// WorkAreaFromScreen converts a Wails v3 Screen into a logical WorkAreaRect.
// Wails v3 Screen.WorkArea is already normalized to logical DIP coordinates on both
// macOS and Windows; if WorkArea is empty we fall back to Bounds or PhysicalWorkArea.
func WorkAreaFromScreen(s *application.Screen) WorkAreaRect {
	if s == nil {
		return WorkAreaRect{Width: 1440, Height: 900, ScaleFactor: 1}
	}
	scale := s.ScaleFactor
	if scale <= 0 {
		scale = 1
	}
	wa := s.WorkArea
	if wa.Width <= 0 || wa.Height <= 0 {
		if s.Bounds.Width > 0 && s.Bounds.Height > 0 {
			wa = s.Bounds
		} else if s.PhysicalWorkArea.Width > 0 && s.PhysicalWorkArea.Height > 0 {
			return PhysicalToLogicalWorkArea(WorkAreaRect{
				X:           s.PhysicalWorkArea.X,
				Y:           s.PhysicalWorkArea.Y,
				Width:       s.PhysicalWorkArea.Width,
				Height:      s.PhysicalWorkArea.Height,
				ScaleFactor: scale,
			})
		}
	}
	return WorkAreaRect{
		X:           wa.X,
		Y:           wa.Y,
		Width:       wa.Width,
		Height:      wa.Height,
		ScaleFactor: scale,
	}
}

// PhysicalToLogicalWorkArea converts raw physical pixel coordinates into logical DIPs
// using the display's DPI ScaleFactor (e.g., 1.25, 1.5, 2.0).
func PhysicalToLogicalWorkArea(phys WorkAreaRect) WorkAreaRect {
	scale := float64(phys.ScaleFactor)
	if scale <= 0 {
		scale = 1
	}
	return WorkAreaRect{
		X:           int(math.Round(float64(phys.X) / scale)),
		Y:           int(math.Round(float64(phys.Y) / scale)),
		Width:       int(math.Round(float64(phys.Width) / scale)),
		Height:      int(math.Round(float64(phys.Height) / scale)),
		ScaleFactor: float32(scale),
	}
}

// DetectAnchorFromWindowPosition inspects a window's WorkArea-relative Y position and height
// and returns AnchorUp when the window is docked near the bottom edge or resides within
// the bottom 28% zone of the active work area.
func DetectAnchorFromWindowPosition(workArea WorkAreaRect, winRelY, winH int) AnchorDirection {
	if workArea.Height <= 0 || winH <= 0 {
		return AnchorDown
	}
	// If the window itself fills almost the entire screen height, stay anchored down.
	if winH >= int(float64(workArea.Height)*0.90) {
		return AnchorDown
	}
	winBottom := winRelY + winH
	bottomGap := workArea.Height - winBottom
	// Flush against the bottom edge (e.g. Windows taskbar system tray) with more space above than below
	if bottomGap <= defaultWindowMargin*3 && winRelY > bottomGap {
		return AnchorUp
	}
	// Or window center sits inside the bottom 28% band
	centerY := winRelY + winH/2
	if float64(centerY) >= float64(workArea.Height)*bottomFlipThresholdRatio {
		return AnchorUp
	}
	return AnchorDown
}

// ComputeWindowPlacementForOS calculates clamped WorkArea-relative & absolute DIP coordinates
// plus the AnchorDirection ("down" or "up") for a given OS, active work area, window size,
// spawn trigger, and optional target point (in absolute logical DIP coordinates).
func ComputeWindowPlacementForOS(
	goos string,
	workArea WorkAreaRect,
	winW, winH int,
	trigger SpawnTrigger,
	targetPoint *application.Point,
	margin int,
) WindowPlacement {
	scale := workArea.ScaleFactor
	if scale <= 0 {
		scale = 1
	}
	if margin < 0 {
		margin = defaultWindowMargin
	}

	effectiveW := winW
	if workArea.Width > 0 && effectiveW > workArea.Width {
		effectiveW = workArea.Width
	}
	effectiveH := winH
	if workArea.Height > 0 && effectiveH > workArea.Height {
		effectiveH = workArea.Height
	}

	clampX := func(x int) int {
		maxX := workArea.Width - effectiveW - margin
		if maxX < margin {
			if workArea.Width > effectiveW {
				return (workArea.Width - effectiveW) / 2
			}
			return 0
		}
		if x < margin {
			return margin
		}
		if x > maxX {
			return maxX
		}
		return x
	}

	clampY := func(y int) int {
		maxY := workArea.Height - effectiveH - margin
		if maxY < margin {
			if workArea.Height > effectiveH {
				return (workArea.Height - effectiveH) / 2
			}
			return 0
		}
		if y < margin {
			return margin
		}
		if y > maxY {
			return maxY
		}
		return y
	}

	anchor := AnchorDown
	var relX, relY int

	switch {
	case targetPoint != nil:
		relTX := targetPoint.X - workArea.X
		relTY := targetPoint.Y - workArea.Y
		inBottomZone := workArea.Height > 0 && float64(relTY) >= float64(workArea.Height)*bottomFlipThresholdRatio

		if inBottomZone {
			anchor = AnchorUp
			if trigger == TriggerTray && relTX >= workArea.Width/2 {
				relX = clampX(workArea.Width - effectiveW - margin)
			} else {
				relX = clampX(relTX - effectiveW/2)
			}
			bottomEdge := relTY
			if bottomEdge > workArea.Height-margin {
				bottomEdge = workArea.Height - margin
			}
			relY = clampY(bottomEdge - effectiveH)
		} else {
			anchor = AnchorDown
			if trigger == TriggerTray && relTX >= workArea.Width/2 {
				relX = clampX(workArea.Width - effectiveW - margin)
			} else {
				relX = clampX(relTX - effectiveW/2)
			}
			relY = clampY(relTY)
		}

	case trigger == TriggerTray && goos == "windows":
		// Windows system tray lives in the bottom-right corner by default -> flip upward!
		anchor = AnchorUp
		relX = clampX(workArea.Width - effectiveW - margin)
		relY = clampY(workArea.Height - effectiveH - margin)

	case trigger == TriggerTray:
		// macOS / top-bar tray default: anchor cleanly below top-right menu bar
		anchor = AnchorDown
		relX = clampX(workArea.Width - effectiveW - margin)
		relY = clampY(margin)

	default:
		// Centered hotkey spawn in upper-middle of the active work area
		anchor = AnchorDown
		relX = clampX((workArea.Width - effectiveW) / 2)
		relY = clampY((workArea.Height - effectiveH) / 2)
	}

	return WindowPlacement{
		RelX:        relX,
		RelY:        relY,
		AbsX:        workArea.X + relX,
		AbsY:        workArea.Y + relY,
		Anchor:      anchor,
		ScaleFactor: scale,
	}
}
