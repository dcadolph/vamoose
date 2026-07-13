//go:build windows

package cmd

import _ "embed"

// trayIcon is the moose mark as an ICO, the format the Windows notification area needs.
//
//go:embed appui/tray.ico
var trayIcon []byte
