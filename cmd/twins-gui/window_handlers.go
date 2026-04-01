package main

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/gui/window"
)

// ==========================================
// Window State Transition Methods
// ==========================================
//
// These methods transition the window between application states.
// Each state has predefined size, constraints, and positioning configured in window.Manager.
//
// State flow: splash (480x550) → main (1024x768)
//
// For basic window operations (show, hide, maximize, minimize, fullscreen, etc.),
// use Wails runtime functions directly from the frontend.

// SetWindowToSplash transitions to splash state (480x550, centered, always-on-top).
// Called by App.tsx:164 after intro completion to show initialization progress.
func (a *App) SetWindowToSplash() error {
	if a.windowManager == nil {
		return fmt.Errorf("window manager not initialized")
	}
	return a.windowManager.TransitionTo(window.StateSplash)
}

// SetWindowToMain transitions to main state (1024x768, resizable, centered).
// Called by App.tsx:189 after splash completion to show the main wallet interface.
func (a *App) SetWindowToMain() error {
	if a.windowManager == nil {
		return fmt.Errorf("window manager not initialized")
	}
	return a.windowManager.TransitionTo(window.StateMain)
}

// SetTrayIconVisible shows or hides the system tray icon at runtime.
// Called by the frontend after applying the fHideTrayIcon setting so the
// change takes effect immediately without a restart.
func (a *App) SetTrayIconVisible(visible bool) {
	if a.trayManager == nil {
		return
	}
	a.trayManager.SetVisible(visible)
}

// HandleWindowMinimized is called by the frontend when the window becomes hidden
// (via visibilitychange event). When fMinimizeToTray is enabled and the tray icon
// is active, hides the window to the system tray instead of the taskbar.
// Matches legacy C++ changeEvent() + QTimer::singleShot(hide) behavior.
func (a *App) HandleWindowMinimized() {
	if a.settingsService == nil || a.trayManager == nil {
		return
	}
	if !a.trayManager.IsStarted() {
		return // Tray not active, can't hide to tray
	}
	if !a.settingsService.GetBool("fMinimizeToTray") {
		return
	}
	// Only apply in main state (not during splash/intro/shutdown)
	if a.windowManager != nil && a.windowManager.GetCurrentState() != window.StateMain {
		return
	}
	a.trayManager.HideWindow()
}
