//go:build !darwin

package main

// dispatchSystrayToMainThread calls startFn directly on non-macOS platforms.
// On Linux and Windows, systray does not require main thread dispatch.
func dispatchSystrayToMainThread(startFn func()) {
	startFn()
}

// hideDockIcon is a no-op on non-macOS platforms.
func hideDockIcon() {}

// showDockIcon is a no-op on non-macOS platforms.
func showDockIcon() {}
