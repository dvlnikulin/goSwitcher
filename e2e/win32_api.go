//go:build windows && e2e

package e2e

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
	"unicode/utf16"
)

// ─────────────────────────────────────────────────────────────
// Win32 API — Lazy DLLs
// ─────────────────────────────────────────────────────────────

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	imm32                   = syscall.NewLazyDLL("imm32.dll")

	procFindWindowW              = user32.NewProc("FindWindowW")
	procFindWindowExW            = user32.NewProc("FindWindowExW")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSwitchToThisWindow       = user32.NewProc("SwitchToThisWindow")
	procGetKeyboardLayout        = kernel32.NewProc("GetKeyboardLayout")
	procGetWindowThreadProcessId = kernel32.NewProc("GetWindowThreadProcessId")
	procLoadKeyboardLayoutW      = kernel32.NewProc("LoadKeyboardLayoutW")
	procSendInput                = user32.NewProc("SendInput")
	procGetMessageExtraInfo      = user32.NewProc("GetMessageExtraInfo")
	procMapVirtualKeyW           = user32.NewProc("MapVirtualKeyW")
	procActivateKeyboardLayout   = kernel32.NewProc("ActivateKeyboardLayout")
	procImmDisableIME            = imm32.NewProc("ImmDisableIME")
)

// ─────────────────────────────────────────────────────────────
// Constants (Win32)
// ─────────────────────────────────────────────────────────────

const (
	INPUT_KEYBOARD    = 1
	KEYEVENTF_KEYUP   = 0x0002
	KEYEVENTF_UNICODE = 0x0004
	KEYEVENTF_SCANCODE = 0x0008
	MAPVK_VK_TO_VSC   = 0
	MAPVK_VK_TO_CHAR  = 2
	MAPVK_VSC_TO_VK   = 3

	// Virtual Key Codes
	VK_SPACE      = 0x20
	VK_RETURN     = 0x0D
	VK_SHIFT      = 0x10
	VK_CONTROL    = 0x11
	VK_MENU       = 0x12
	VK_BACK       = 0x08
	VK_DELETE     = 0x2E
	VK_LEFT       = 0x25
	VK_RIGHT      = 0x27
	VK_UP         = 0x26
	VK_DOWN       = 0x28
	VK_TAB        = 0x09
	VK_HOME       = 0x24
	VK_END        = 0x23
	VK_INSERT     = 0x2D
	VK_ESCAPE     = 0x1B
	VK_LSHIFT     = 0xA0
	VK_RSHIFT     = 0xA1
	VK_LCONTROL   = 0xA2
	VK_RCONTROL   = 0xA3
	VK_LMENU      = 0xA4
	VK_RMENU      = 0xA5
	VK_CAPITAL    = 0x14
	VK_NUMLOCK    = 0x90
	VK_SCROLLLOCK = 0x91

	// Keyboard Layout IDs
	KLID_EN_US = "00000409"
	KLID_RU_RU = "00000419"
	KLID_DE_DE = "00000407"

	// WM Messages
	WM_GETTEXT       = 0x000D
	WM_GETTEXTLENGTH = 0x000E
	WM_SETTEXT       = 0x000C
)

// ─────────────────────────────────────────────────────────────
// INPUT structures (Win32 union)
// ─────────────────────────────────────────────────────────────

type KEYBDINPUT struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	DwExtraInfo uintptr
}

type MOUSEINPUT struct {
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	DwExtraInfo uintptr
}

type HARDWAREINPUT struct {
	UMsg      uint32
	ParamsL   uint16
	ParamsH   uint16
}

type INPUT struct {
	Type uint32
	Ki   KEYBDINPUT
	Mi   MOUSEINPUT
	Hi   HARDWAREINPUT
}

// ─────────────────────────────────────────────────────────────
// SendInput wrapper
// ─────────────────────────────────────────────────────────────

// sendInputRaw — низкий уровень SendInput
func sendInputRaw(inputs []INPUT) uint {
	if len(inputs) == 0 {
		return 0
	}
	ret, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(unsafe.Sizeof(INPUT{})),
	)
	return uint(ret)
}

// ─────────────────────────────────────────────────────────────
// Launch & Window Management
// ─────────────────────────────────────────────────────────────

// launchTargetWindows — запускает Notepad и находит EDIT контрол
func launchTargetWindows() (uintptr, uintptr) {
	// Запускаем notepad.exe
	cmd := exec.Command("notepad.exe")
	if err := cmd.Start(); err != nil {
		return 0, 0
	}

	// Ждём появления окна Notepad
	var notepadHwnd uint32
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		notepadHwnd = findWindow("Notepad", "")
		if notepadHwnd != 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if notepadHwnd == 0 {
		// Не удалось найти Notepad — пробуем без класса
		// На некоторых системах класс окна отличается
		fmt.Println("[E2E] WARNING: Notepad не найден по классу")
		return 0, 0
	}

	// Находим EDIT контрол
	var editHwnd uint32
	deadline2 := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline2) {
		editHwnd = findWindowEx(notepadHwnd, "Edit", "")
		if editHwnd != 0 {
			break
		}
		// Попробуем другие классы
		editHwnd = findWindowEx(notepadHwnd, "RICHEDIT50W", "")
		if editHwnd != 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if editHwnd == 0 {
		fmt.Println("[E2E] WARNING: EDIT контрол не найден")
		return uintptr(notepadHwnd), 0
	}

	// Фокусируем
	setForegroundWindow(editHwnd)
	time.Sleep(300 * time.Millisecond)

	// Очищаем
	clearText(editHwnd)

	return uintptr(notepadHwnd), uintptr(editHwnd)
}

// findWindow — обёртка над FindWindowW
func findWindow(className, windowName string) uint32 {
	var cls, win *uint16
	if className != "" {
		cls, _ = syscall.UTF16PtrFromString(className)
	}
	if windowName != "" {
		win, _ = syscall.UTF16PtrFromString(windowName)
	}
	ret, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(win)))
	return uint32(ret)
}

// findWindowEx — обёртка над FindWindowExW
func findWindowEx(parent uint32, className, windowName string) uint32 {
	var cls, win *uint16
	if className != "" {
		cls, _ = syscall.UTF16PtrFromString(className)
	}
	if windowName != "" {
		win, _ = syscall.UTF16PtrFromString(windowName)
	}
	ret, _, _ := procFindWindowExW.Call(uintptr(parent), 0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(win)))
	return uint32(ret)
}

// setForegroundWindow — обёртка над SetForegroundWindow
func setForegroundWindow(hwnd uint32) bool {
	ret, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	return ret != 0
}

// ─────────────────────────────────────────────────────────────
// Text Manipulation
// ─────────────────────────────────────────────────────────────

// clearText — очищает текст в EDIT контроле (Ctrl+A, Delete)
func clearText(hwnd uint32) {
	// Ctrl+A
	SendKeyCombo([]uint16{VK_CONTROL, 'A'}, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	// Delete
	SendTestKey(VK_DELETE, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
}

// getEditText — получает текст из EDIT контрола через WM_GETTEXT
func getEditText(hwnd uint32) string {
	// Сначала получаем длину текста
	lenRet := sendMessage(hwnd, WM_GETTEXTLENGTH, 0, 0)
	length := int(lenRet) + 1

	buf := make([]uint16, length)
	ret := sendMessage(hwnd, WM_GETTEXT, uintptr(length), uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		return ""
	}
	return string(utf16.Decode(buf[:ret]))
}

// ─────────────────────────────────────────────────────────────
// Keyboard Layout
// ─────────────────────────────────────────────────────────────

// EnsureEnglishLayout — гарантирует раскладку EN для окна
func EnsureEnglishLayout(hwnd uintptr) {
	hwnd32 := uint32(hwnd)
	currentKL := getKeyboardLayoutID(hwnd32)
	if currentKL != KLID_EN_US {
		fmt.Printf("[E2E] Смена раскладки с %s на EN\n", currentKL)
		setKeyboardLayout(hwnd32, KLID_EN_US)
		time.Sleep(LayoutSwitchDelay)
	}
}

// getKeyboardLayoutID — получает ID раскладки окна
func getKeyboardLayoutID(hwnd uint32) string {
	threadID, _, _ := procGetWindowThreadProcessId.Call(uintptr(hwnd), 0)
	layout, _, _ := procGetKeyboardLayout.Call(threadID)
	return fmt.Sprintf("%08X", layout&0xFFFF0000)
}

// setKeyboardLayout — устанавливает раскладку для потока
func setKeyboardLayout(hwnd uint32, klid string) bool {
	klidW, _ := syscall.UTF16FromString(klid)
	ret, _, _ := procLoadKeyboardLayoutW.Call(
		uintptr(unsafe.Pointer(&klidW[0])),
		1, // KLF_ACTIVATE
	)
	return ret != 0
}

// ─────────────────────────────────────────────────────────────
// Message Sending
// ─────────────────────────────────────────────────────────────

func sendMessage(hwnd uint32, msg, wParam, lParam uintptr) uintptr {
	ret, _, _ := procSendMessageW.Call(uintptr(hwnd), msg, wParam, lParam)
	return ret
}
