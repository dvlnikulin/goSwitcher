//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var lockFile *os.File

func defaultBlacklist() string {
	return "zed-editor\ncode\nkonsole\nalacritty\nkitty\nwinbox\nvmware\nvirtualbox"
}

func enforceSingleInstance() {
	f, err := os.OpenFile("/tmp/goswitcher.lock", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		os.Exit(0)
	}
	lockFile = f
}

func setSystemAutostart(enable bool) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	autostartDir := filepath.Join(configDir, "autostart")
	desktopFile := filepath.Join(autostartDir, "goswitcher.desktop")

	if !enable {
		os.Remove(desktopFile)
		return
	}

	os.MkdirAll(autostartDir, 0755)
	exe, _ := os.Executable()
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=GoSwitcher
Comment=AI Keyboard Layout Switcher
Exec=%s
Icon=input-keyboard
Terminal=false
Categories=Utility;
X-GNOME-Autostart-enabled=true
`, exe)
	os.WriteFile(desktopFile, []byte(content), 0644)
}

func openBrowser(url string) {
	exec.Command("xdg-open", url).Start()
}
