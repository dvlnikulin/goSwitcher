//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func defaultBlacklist() string {
	return "zed.exe\ncode.exe\nwindowsterminal.exe\nwinbox.exe\nvmware.exe\nvirtualbox.exe\ngenshinimpact.exe\nbg3.exe\nbg3_dx11.exe"
}

func enforceSingleInstance() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex := kernel32.NewProc("CreateMutexW")
	mutexName, _ := syscall.UTF16PtrFromString("GoSwitcher_Unique_Mutex")
	handle, _, err := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if handle == 0 || err == syscall.Errno(183) {
		os.Exit(0)
	}
}

func setSystemAutostart(enable bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
	if err == nil {
		defer k.Close()
		exe, _ := os.Executable()
		if enable {
			k.SetStringValue("GoSwitcher", `"`+exe+`"`)
		} else {
			k.DeleteValue("GoSwitcher")
		}
	}
}

func openBrowser(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
