//go:build linux

package cmd

import _ "embed"

// trayIcon is the moose mark as a PNG, which the StatusNotifierItem protocol accepts.
//
//go:embed appui/mark.png
var trayIcon []byte
