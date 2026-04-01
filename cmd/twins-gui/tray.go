package main

import (
	"sync"
	"sync/atomic"

	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// transparentPNG is a minimal 1x1 transparent RGBA PNG used to visually hide
// the tray icon. systray.Quit() cannot be used because it terminates the app,
// so we swap to this invisible icon instead.
var transparentPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, // RGBA, CRC
	0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41, 0x54, // IDAT chunk (len=11)
	0x78, 0x9c, 0x63, 0x60, 0x00, 0x02, 0x00, 0x00, 0x05, 0x00, 0x01, // zlib compressed RGBA 0,0,0,0
	0x7a, 0x5e, 0xab, 0x3f, // CRC
	0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, // IEND chunk
	0xae, 0x42, 0x60, 0x82, // CRC
}

// TrayManager manages the system tray icon and context menu.
// It wraps energye/systray and coordinates with the Wails window lifecycle.
type TrayManager struct {
	app          *App
	windowHidden atomic.Bool // true when window is hidden to tray
	started      atomic.Bool // true after systray.Run has been called
	iconHidden   atomic.Bool // true when icon is visually hidden via transparent PNG
	stopOnce     sync.Once
	endFunc      func() // cleanup function from RunWithExternalLoop
	icon         []byte // cached icon bytes for restore after SetVisible
}

// NewTrayManager creates a new tray manager bound to the given app.
func NewTrayManager(app *App) *TrayManager {
	return &TrayManager{app: app}
}

// Start initializes the system tray icon with the given icon bytes.
// Must be called after the Wails event loop is running (e.g., from domReady).
func (t *TrayManager) Start(icon []byte) {
	if t.started.Swap(true) {
		return // already started
	}
	t.icon = icon

	start, end := systray.RunWithExternalLoop(func() {
		// onReady - called when systray is initialized.
		// Check iconHidden: if SetVisible(false) was called before onReady
		// fired (async on macOS main thread), start with transparent icon.
		if t.iconHidden.Load() {
			systray.SetIcon(transparentPNG)
			systray.SetTooltip("")
		} else {
			systray.SetIcon(icon)
			systray.SetTooltip("TWINS Core Wallet")
		}

		mToggle := systray.AddMenuItem("Show/Hide TWINS Core", "Toggle window visibility")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit TWINS Core")

		mToggle.Click(func() {
			t.ToggleWindow()
		})

		mQuit.Click(func() {
			t.handleQuit()
		})

		// On macOS, the menu must be explicitly attached to the status item.
		// Without this call, clicking the tray icon shows nothing.
		systray.CreateMenu()
	}, func() {
		// onExit - cleanup
	})

	t.endFunc = end
	// On macOS, NSStatusItem must be created on the main thread.
	// dispatchSystrayToMainThread handles this via dispatch_async on darwin,
	// and calls directly on other platforms.
	dispatchSystrayToMainThread(start)
}

// Stop tears down the system tray icon. Idempotent via sync.Once.
func (t *TrayManager) Stop() {
	t.stopOnce.Do(func() {
		if t.started.Load() {
			systray.Quit()
			if t.endFunc != nil {
				t.endFunc()
			}
		}
	})
}

// ToggleWindow shows the window if hidden, hides it if visible.
// Matches legacy C++ toggleHidden() behavior.
func (t *TrayManager) ToggleWindow() {
	if t.windowHidden.Load() {
		t.ShowWindow()
	} else {
		t.HideWindow()
	}
}

// ShowWindow restores the window from tray-hidden state.
// On macOS, restores the Dock icon and menu bar before showing the window.
func (t *TrayManager) ShowWindow() {
	if t.app.ctx == nil {
		return
	}
	showDockIcon()
	runtime.WindowShow(t.app.ctx)
	runtime.WindowUnminimise(t.app.ctx)
	t.windowHidden.Store(false)
}

// HideWindow hides the window to the system tray.
// On macOS, also hides the Dock icon and menu bar so the tray icon is the
// only way to restore the window.
func (t *TrayManager) HideWindow() {
	if t.app.ctx == nil {
		return
	}
	runtime.WindowHide(t.app.ctx)
	t.windowHidden.Store(true)
	hideDockIcon()
}

// IsWindowHidden returns true when the window is hidden in the tray.
func (t *TrayManager) IsWindowHidden() bool {
	return t.windowHidden.Load()
}

// IsStarted returns true when the tray icon has been initialized.
func (t *TrayManager) IsStarted() bool {
	return t.started.Load()
}

// SetVisible controls tray icon visibility. Since systray.Quit() terminates
// the application, we swap the icon to a 1x1 transparent PNG to hide it and
// restore the real icon to show it. When hiding while the window is
// hidden-to-tray, the window is shown first to prevent an inaccessible state.
func (t *TrayManager) SetVisible(show bool) {
	if !t.started.Load() {
		return // tray not initialized
	}
	if show {
		if !t.iconHidden.Load() {
			return // already visible
		}
		systray.SetIcon(t.icon)
		systray.SetTooltip("TWINS Core Wallet")
		t.iconHidden.Store(false)
	} else {
		if t.iconHidden.Load() {
			return // already hidden
		}
		if t.windowHidden.Load() {
			// Show window before hiding tray to prevent inaccessible state.
			t.ShowWindow()
		}
		systray.SetIcon(transparentPNG)
		systray.SetTooltip("")
		t.iconHidden.Store(true)
	}
}

// IsIconHidden returns true when the tray icon is visually hidden.
func (t *TrayManager) IsIconHidden() bool {
	return t.iconHidden.Load()
}

// handleQuit initiates application quit from the tray context menu.
// Shows the window first so the shutdown dialog is visible, then
// triggers the standard shutdown flow.
func (t *TrayManager) handleQuit() {
	if t.app.ctx == nil {
		return
	}
	// Show window so shutdown UI is visible
	if t.windowHidden.Load() {
		t.ShowWindow()
	}
	// Set flag so OnBeforeClose skips minimize behaviors
	t.app.trayQuitRequested.Store(true)
	runtime.Quit(t.app.ctx)
}
