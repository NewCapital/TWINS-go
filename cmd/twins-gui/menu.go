package main

import (
	"runtime"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

// Tools Window tab indices
const (
	ToolsTabInformation    = 0
	ToolsTabConsole        = 1
	ToolsTabNetworkTraffic = 2
	ToolsTabPeers          = 3
	ToolsTabWalletRepair   = 4
)

// CreateApplicationMenu creates the native menu bar for the application
// Uses platform-specific menu placement following desktop conventions
func (a *App) CreateApplicationMenu() *menu.Menu {
	appMenu := menu.NewMenu()

	if runtime.GOOS == "darwin" {
		a.addMacOSMenus(appMenu)
	} else {
		a.addWindowsLinuxMenus(appMenu)
	}

	return appMenu
}

// addMacOSMenus adds macOS-specific menus following Apple HIG
// Preferences goes in the app menu with ⌘, shortcut
func (a *App) addMacOSMenus(appMenu *menu.Menu) {
	// App menu (TWINS)
	twinsMenu := appMenu.AddSubmenu("TWINS")
	twinsMenu.AddText("About TWINS", nil, a.handleAbout)
	twinsMenu.AddSeparator()
	twinsMenu.AddText("Preferences...", keys.CmdOrCtrl(","), a.handlePreferences)
	twinsMenu.AddSeparator()
	twinsMenu.AddText("Hide TWINS", keys.CmdOrCtrl("h"), nil)
	twinsMenu.AddText("Hide Others", keys.Combo("h", keys.CmdOrCtrlKey, keys.OptionOrAltKey), nil)
	twinsMenu.AddText("Show All", nil, nil)
	twinsMenu.AddSeparator()
	twinsMenu.AddText("Quit TWINS", keys.CmdOrCtrl("q"), a.handleQuit)

	// File menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Backup Wallet...", nil, a.handleBackupWallet)
	fileMenu.AddText("Sign/Verify Message...", nil, a.handleSignVerifyMessage)
	fileMenu.AddText("Sending Addresses...", nil, a.handleAddressBook)

	// Edit menu (standard)
	appMenu.Append(menu.EditMenu())

	// Tools menu
	toolsMenu := appMenu.AddSubmenu("Tools")
	toolsMenu.AddText("Information", keys.CmdOrCtrl("d"), a.handleToolsInformation)
	toolsMenu.AddText("Console", nil, a.handleToolsConsole)
	toolsMenu.AddText("Network Traffic", nil, a.handleToolsNetworkTraffic)
	toolsMenu.AddText("Peers", nil, a.handleToolsPeers)
	toolsMenu.AddText("Wallet Repair", nil, a.handleToolsWalletRepair)

	// Window menu (standard)
	windowMenu := appMenu.AddSubmenu("Window")
	windowMenu.AddText("Minimize", keys.CmdOrCtrl("m"), nil)
	windowMenu.AddText("Zoom", nil, nil)
	windowMenu.AddSeparator()
	windowMenu.AddText("Bring All to Front", nil, nil)
}

// addWindowsLinuxMenus adds Windows/Linux-specific menus
// Preferences goes in Edit menu with Ctrl+, shortcut
func (a *App) addWindowsLinuxMenus(appMenu *menu.Menu) {
	// File menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Backup Wallet...", nil, a.handleBackupWallet)
	fileMenu.AddText("Sign/Verify Message...", nil, a.handleSignVerifyMessage)
	fileMenu.AddText("Sending Addresses...", nil, a.handleAddressBook)
	fileMenu.AddSeparator()
	fileMenu.AddText("Exit", keys.CmdOrCtrl("q"), a.handleQuit)

	// Edit menu with Preferences
	editMenu := appMenu.AddSubmenu("Edit")
	editMenu.AddText("Preferences...", keys.CmdOrCtrl(","), a.handlePreferences)
	editMenu.AddSeparator()
	editMenu.AddText("Undo", keys.CmdOrCtrl("z"), nil)
	editMenu.AddText("Redo", keys.CmdOrCtrl("y"), nil)
	editMenu.AddSeparator()
	editMenu.AddText("Cut", keys.CmdOrCtrl("x"), nil)
	editMenu.AddText("Copy", keys.CmdOrCtrl("c"), nil)
	editMenu.AddText("Paste", keys.CmdOrCtrl("v"), nil)
	editMenu.AddSeparator()
	editMenu.AddText("Select All", keys.CmdOrCtrl("a"), nil)

	// Tools menu
	toolsMenu := appMenu.AddSubmenu("Tools")
	toolsMenu.AddText("Information", keys.CmdOrCtrl("d"), a.handleToolsInformation)
	toolsMenu.AddText("Console", nil, a.handleToolsConsole)
	toolsMenu.AddText("Network Traffic", nil, a.handleToolsNetworkTraffic)
	toolsMenu.AddText("Peers", nil, a.handleToolsPeers)
	toolsMenu.AddText("Wallet Repair", nil, a.handleToolsWalletRepair)

	// Help menu
	helpMenu := appMenu.AddSubmenu("Help")
	helpMenu.AddText("About TWINS", nil, a.handleAbout)
}

// handlePreferences opens the Options/Preferences dialog
func (a *App) handlePreferences(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-preferences")
}

// handleAbout shows the About dialog
func (a *App) handleAbout(_ *menu.CallbackData) {
	wailsRuntime.MessageDialog(a.ctx, wailsRuntime.MessageDialogOptions{
		Type:    wailsRuntime.InfoDialog,
		Title:   "About TWINS Wallet",
		Message: "TWINS Core Wallet\nModern Go Implementation\n\nBuilt with Wails",
	})
}

// handleToolsInformation opens the Tools Window to the Information tab
func (a *App) handleToolsInformation(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-tools-window", ToolsTabInformation)
}

// handleToolsConsole opens the Tools Window to the Console tab
func (a *App) handleToolsConsole(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-tools-window", ToolsTabConsole)
}

// handleToolsNetworkTraffic opens the Tools Window to the Network Traffic tab
func (a *App) handleToolsNetworkTraffic(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-tools-window", ToolsTabNetworkTraffic)
}

// handleToolsPeers opens the Tools Window to the Peers tab
func (a *App) handleToolsPeers(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-tools-window", ToolsTabPeers)
}

// handleToolsWalletRepair opens the Tools Window to the Wallet Repair tab
func (a *App) handleToolsWalletRepair(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-tools-window", ToolsTabWalletRepair)
}

// handleBackupWallet opens a save dialog and backs up the wallet file
func (a *App) handleBackupWallet(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:backup-wallet")
}

// handleSignVerifyMessage opens the Sign/Verify Message dialog
func (a *App) handleSignVerifyMessage(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-sign-verify")
}

// handleAddressBook opens the Address Book dialog
func (a *App) handleAddressBook(_ *menu.CallbackData) {
	wailsRuntime.EventsEmit(a.ctx, "menu:open-address-book")
}

// handleQuit exits the application
func (a *App) handleQuit(_ *menu.CallbackData) {
	wailsRuntime.Quit(a.ctx)
}
