package main

import (
	"embed"
	"strings"
)

// Build-time injectable variables via -ldflags:
//
//	-X main.AppName=Vex
//	-X main.Edition=enterprise
//	-X main.AppVersion=0.1.0
//	-X main.UpdateFeedURL=https://intranet.example.com/vex/manifest.json
var (
	AppName       = "Vex"
	Edition       = "community"
	AppVersion    = "0.1.0"
	UpdateFeedURL = ""
)

// Optional 16x16 tray icon override at assets/tray_icon.png (empty by default).
//
//go:embed assets/*
var embeddedAssets embed.FS

// AppInfo summarizes build-time identity for the frontend UI and diagnostics.
type AppInfo struct {
	Name    string `json:"name"`
	Edition string `json:"edition"`
	Version string `json:"version"`
}

func currentAppInfo() AppInfo {
	name := strings.TrimSpace(AppName)
	if name == "" {
		name = "Vex"
	}
	ed := strings.TrimSpace(Edition)
	if ed == "" {
		ed = "community"
	}
	ver := strings.TrimSpace(AppVersion)
	if ver == "" {
		ver = "0.1.0"
	}
	return AppInfo{
		Name:    name,
		Edition: ed,
		Version: ver,
	}
}

func trayIconBytes() []byte {
	data, err := embeddedAssets.ReadFile("assets/tray_icon.png")
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}
