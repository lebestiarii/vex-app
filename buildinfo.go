package main

import (
	"embed"
	"strings"
)

// Build-time injectable variables via -ldflags:
//
//	-X main.AppName=Vex
//	-X main.AppVersion=0.2.0
//	-X main.UpdateFeedURL=https://updates.example.com/vex/manifest.json
var (
	AppName       = "Vex"
	AppVersion    = "0.2.0"
	UpdateFeedURL = ""
)

// Optional 16x16 tray icon override at assets/tray_icon.png (empty by default).
//
//go:embed assets/*
var embeddedAssets embed.FS

// AppInfo summarizes build-time identity for the frontend UI and diagnostics.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func currentAppInfo() AppInfo {
	name := strings.TrimSpace(AppName)
	if name == "" {
		name = "Vex"
	}
	ver := strings.TrimSpace(AppVersion)
	if ver == "" {
		ver = "0.2.0"
	}
	return AppInfo{
		Name:    name,
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
