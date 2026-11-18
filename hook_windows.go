//go:build windows

package main

import (
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                        = windows.NewLazySystemDLL("user32.dll")
	kernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHookEx          = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx            = user32.NewProc("CallNextHookEx")
	procGetMessage                = user32.NewProc("GetMessageW")
	procPostMessage               = user32.NewProc("PostMessageW")
	procGetForeground             = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId  = user32.NewProc("GetWindowThreadProcessId")
	procGetKeyboardLayout         = user32.NewProc("GetKeyboardLayout")
	procGetKeyboardLayoutList     = user32.NewProc("GetKeyboardLayoutList")
	procGetKeyState               = user32.NewProc("GetKeyState")
	procSendInput                 = user32.NewProc("SendInput")
	procOpenClipboard             = user32.NewProc("OpenClipboard")
	procCloseClipboard            = user32.NewProc("CloseClipboard")
	procEmptyClipboard            = user32.NewProc("EmptyClipboard")
	procGetClipboardData          = user32.NewProc("GetClipboardData")
	procSetClipboardData          = user32.NewProc("SetClipboardData")
	procOpenProcess               = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
	procGlobalAlloc               = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                = kernel32.NewProc("GlobalLock")
	procGlobalUnlock              = kernel32.NewProc("GlobalUnlock")

	keyboardHook uintptr
)

const (
	WH_KEYBOARD_LL                    = 13
	WM_KEYDOWN                        = 0x0100
	WM_KEYUP                          = 0x0101
	WM_SYSKEYDOWN                     = 0x0104
	WM_SYSKEYUP                       = 0x0105
	WM_INPUTLANGCHANGEREQUEST         = 0x0050
	VK_BACK                           = 0x08
	VK_TAB                            = 0x09
	VK_RETURN                         = 0x0D
	VK_SHIFT                          = 0x10
	VK_CONTROL                        = 0x11
	VK_MENU                           = 0x12
	VK_CAPITAL                        = 0x14
	VK_SPACE                          = 0x20
	VK_LSHIFT                         = 0xA0
	VK_RSHIFT                         = 0xA1
	VK_LCONTROL                       = 0xA2
	VK_RCONTROL                       = 0xA3
	VK_LMENU                          = 0xA4
	VK_RMENU                          = 0xA5
	INPUT_KEYBOARD                    = 1
	KEYEVENTF_KEYUP                   = 0x0002
	KEYEVENTF_UNICODE                 = 0x0004
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	CF_UNICODETEXT                    = 13
	GMEM_MOVEABLE                     = 0x0002
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	_    uint32
	Ki   KEYBDINPUT
	_    [8]byte
}

var (
	lastPid         uint32
	lastProcessName string
	shiftReleased   bool = true
)

var vkToEn = map[uint32]string{
	48: "0", 49: "1", 50: "2", 51: "3", 52: "4", 53: "5", 54: "6", 55: "7", 56: "8", 57: "9",
	65: "a", 66: "b", 67: "c", 68: "d", 69: "e", 70: "f", 71: "g", 72: "h", 73: "i", 74: "j", 75: "k", 76: "l", 77: "m", 78: "n", 79: "o", 80: "p", 81: "q", 82: "r", 83: "s", 84: "t", 85: "u", 86: "v", 87: "w", 88: "x", 89: "y", 90: "z",
	186: ";", 187: "=", 188: ",", 189: "-", 190: ".", 191: "/", 192: "`", 219: "[", 220: `\`, 221: "]", 222: "'",
}
var vkToRu = map[uint32]string{
	48: "0", 49: "1", 50: "2", 51: "3", 52: "4", 53: "5", 54: "6", 55: "7", 56: "8", 57: "9",
	65: "ф", 66: "и", 67: "с", 68: "в", 69: "у", 70: "а", 71: "п", 72: "р", 73: "ш", 74: "о", 75: "л", 76: "д", 77: "ь", 78: "т", 79: "щ", 80: "з", 81: "й", 82: "к", 83: "ы", 84: "е", 85: "г", 86: "м", 87: "ц", 88: "ч", 89: "н", 90: "я",
	186: "ж", 187: "=", 188: "б", 189: "-", 190: "ю", 191: ".", 192: "ё", 219: "х", 220: `\`, 221: "ъ", 222: "э",
}
var vkToEnShift = map[uint32]string{
	48: ")", 49: "!", 50: "@", 51: "#", 52: "$", 53: "%", 54: "^", 55: "&", 56: "*", 57: "(",
	186: ":", 187: "+", 188: "<", 189: "_", 190: ">", 191: "?", 192: "~", 219: "{", 220: "|", 221: "}", 222: `"`,
}
var vkToRuShift = map[uint32]string{
	48: ")", 49: "!", 50: `"`, 51: "№", 52: ";", 53: "%", 54: ":", 55: "?", 56: "*", 57: "(",
	186: "Ж", 187: "+", 188: "Б", 189: "_", 190: "Ю", 191: ",", 192: "Ё", 219: "Х", 220: "/", 221: "Ъ", 222: "Э",
}

// --- РЕАЛИЗАЦИЯ ИНТЕРФЕЙСОВ ДЛЯ engine.go ---

func osSimulateBackspace() {
	simulateBackspace()
}

func osSendString(text string) {
	sendUnicodeString(text)
}

func osSwitchLayout() {
	hwnd, _, _ := procGetForeground.Call()
	if hwnd != 0 {
		switchLayout(hwnd)
	}
}

func osIsRussianLayout() bool {
	hwnd, _, _ := procGetForeground.Call()
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
	return (layoutId & 0xFFFF) == 0x0419
}

func osGetClipboard() (string, bool) {
	return getClipboardText()
}

func osSetClipboard(text string) bool {
	return setClipboardText(text)
}

func osSendCtrlKey(key string) {
	if key == "c" {
		sendCtrlKey(0x43)
	} else if key == "v" {
		sendCtrlKey(0x56)
	}
}

// --- СИСТЕМНЫЕ ФУНКЦИИ ОС ---

func getClipboardText() (string, bool) {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return "", false
	}
	defer procCloseClipboard.Call()

	hMem, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if hMem == 0 {
		return "", false
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(hMem)

	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr))), true
}

func setClipboardText(text string) bool {
	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return false
	}
	size := uintptr(len(utf16) * 2)

	hMem, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, size)
	if hMem == 0 {
		return false
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return false
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	procGlobalUnlock.Call(hMem)

	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return false
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	res, _, _ := procSetClipboardData.Call(CF_UNICODETEXT, hMem)
	return res != 0
}

func sendKeyCombo(vk uint16, keyUp bool) {
	flags := uint32(0)
	if keyUp {
		flags = KEYEVENTF_KEYUP
	}
	input := INPUT{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{WVk: vk, DwFlags: flags}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func sendCtrlKey(vk uint16) {
	sendKeyCombo(VK_SHIFT, true)
	sendKeyCombo(VK_MENU, true)
	time.Sleep(10 * time.Millisecond)
	sendKeyCombo(VK_CONTROL, false)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(vk, false)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(vk, true)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(VK_CONTROL, true)
}

func getCharEn(vk uint32, shift bool) string {
	char, ok := vkToEn[vk]
	if !ok {
		return ""
	}
	if shift {
		if val, has := vkToEnShift[vk]; has {
			return val
		}
		if vk >= 65 && vk <= 90 {
			return strings.ToUpper(char)
		}
	}
	return char
}

func getProcessName(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(uintptr(PROCESS_QUERY_LIMITED_INFORMATION), 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)
	var size uint32 = 1024
	buf := make([]uint16, size)
	ret, _, _ := procQueryFullProcessImageName.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	path := syscall.UTF16ToString(buf[:size])
	parts := strings.Split(path, "\\")
	if len(parts) > 0 {
		return strings.ToLower(parts[len(parts)-1])
	}
	return ""
}

func sendUnicodeChar(char uint16, keyUp bool) {
	flags := uint32(KEYEVENTF_UNICODE)
	if keyUp {
		flags |= KEYEVENTF_KEYUP
	}
	input := INPUT{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{WVk: 0, WScan: char, DwFlags: flags}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func sendUnicodeString(text string) {
	for _, r := range text {
		if r > 0xFFFF {
			r -= 0x10000
			hi := uint16(0xD800 + (r >> 10))
			lo := uint16(0xDC00 + (r & 0x3FF))
			sendUnicodeChar(hi, false)
			sendUnicodeChar(hi, true)
			sendUnicodeChar(lo, false)
			sendUnicodeChar(lo, true)
		} else {
			sendUnicodeChar(uint16(r), false)
			sendUnicodeChar(uint16(r), true)
		}
	}
}

func simulateBackspace() {
	input := INPUT{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{WVk: VK_BACK, DwFlags: 0}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	input.Ki.DwFlags = KEYEVENTF_KEYUP
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func switchLayout(hwnd uintptr) {
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	currentHkl, _, _ := procGetKeyboardLayout.Call(threadId)
	count, _, _ := procGetKeyboardLayoutList.Call(0, 0)
	if count <= 1 {
		return
	}
	hkls := make([]uintptr, count)
	procGetKeyboardLayoutList.Call(count, uintptr(unsafe.Pointer(&hkls[0])))
	var nextHkl uintptr
	for i, h := range hkls {
		if h == currentHkl {
			nextHkl = hkls[(i+1)%len(hkls)]
			break
		}
	}
	if nextHkl == 0 {
		nextHkl = hkls[0]
	}
	procPostMessage.Call(hwnd, WM_INPUTLANGCHANGEREQUEST, 0, nextHkl)
}

func parseHotkey(hotkeyStr string) (vk uint32, needShift bool, needAlt bool) {
	s := strings.ToLower(strings.TrimSpace(hotkeyStr))
	if strings.Contains(s, "shift+") {
		needShift = true
		s = strings.ReplaceAll(s, "shift+", "")
	}
	if strings.Contains(s, "alt+") {
		needAlt = true
		s = strings.ReplaceAll(s, "alt+", "")
	}
	switch s {
	case "pause":
		vk = 0x13
	case "f12":
		vk = 0x7B
	case "f9":
		vk = 0x78
	case "f8":
		vk = 0x77
	case "scrolllock":
		vk = 0x91
	case "capslock":
		vk = 0x14
	default:
		vk = 0x13
	}
	return
}

func isModifier(vk uint32) bool {
	switch vk {
	case 0x10, 0xA0, 0xA1, 0x11, 0xA2, 0xA3, 0x12, 0xA4, 0xA5, 0x14, 0x5B, 0x5C:
		return true
	}
	return false
}

// --- ЯДРО ХУКА (ПЕРЕДАЧА В ENGINE) ---

func startHook() {
	callback := syscall.NewCallback(hookCallback)
	hook, _, _ := procSetWindowsHookEx.Call(WH_KEYBOARD_LL, callback, 0, 0)
	keyboardHook = hook
	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}
	for {
		procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
	}
}

func hookCallback(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && atomic.LoadUint32(&isReplacing) == 0 {
		kbd := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		vk := kbd.VkCode

		if wParam == WM_KEYUP || wParam == WM_SYSKEYUP {
			if vk == VK_SHIFT || vk == VK_LSHIFT || vk == VK_RSHIFT {
				shiftReleased = true
			}
		} else if wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN {
			hwnd, _, _ := procGetForeground.Call()
			var pid uint32
			procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

			if lastPid != pid {
				if lastPid != 0 {
					EngineClearContext()
				}
				lastProcessName = getProcessName(pid)
				lastPid = pid
			}

			settingsMutex.RLock()
			blacklist := strings.ToLower(settings.Blacklist)
			targetHotkeyVK, targetNeedShift, targetNeedAlt := parseHotkey(settings.Hotkey)
			caseHotkeyVK, caseNeedShift, caseNeedAlt := parseHotkey(settings.CaseHotkey)
			settingsMutex.RUnlock()

			isBlacklisted := false
			for _, b := range strings.Split(blacklist, "\n") {
				b = strings.TrimSpace(b)
				if b != "" && lastProcessName == b {
					isBlacklisted = true
					break
				}
			}

			if isBlacklisted {
				ret, _, _ := procCallNextHookEx.Call(keyboardHook, uintptr(nCode), wParam, lParam)
				return ret
			}

			if vk == VK_SHIFT || vk == VK_LSHIFT || vk == VK_RSHIFT {
				if shiftReleased {
					EngineHandleDoubleShift()
					shiftReleased = false
				}
			} else if vk != VK_BACK {
				EngineResetCounters()
			}

			shiftState, _, _ := procGetKeyState.Call(uintptr(VK_SHIFT))
			altState, _, _ := procGetKeyState.Call(uintptr(VK_MENU))
			ctrlState, _, _ := procGetKeyState.Call(uintptr(VK_CONTROL))
			isShift := (shiftState & 0x8000) != 0
			isAlt := (altState & 0x8000) != 0
			isCtrl := (ctrlState & 0x8000) != 0

			if isCtrl && vk != targetHotkeyVK && vk != caseHotkeyVK {
				ret, _, _ := procCallNextHookEx.Call(keyboardHook, uintptr(nCode), wParam, lParam)
				return ret
			}

			if vk == caseHotkeyVK && isShift == caseNeedShift && isAlt == caseNeedAlt {
				go EngineHandleCaseToggle()
				return 1
			}

			if vk == targetHotkeyVK && isShift == targetNeedShift && isAlt == targetNeedAlt {
				go EngineHandleManualSwitch()
				return 1
			}

			if vk == VK_SPACE {
				EngineHandleSpace()
			} else if vk == VK_RETURN {
				EngineHandleEnter()
			} else if vk == VK_TAB {
				EngineHandleTab()
			} else if vk == VK_BACK {
				EngineHandleBackspace()
			} else {
				charEn := getCharEn(vk, isShift)
				charRu := vkToRu[vk]
				if isShift {
					if val, ok := vkToRuShift[vk]; ok {
						charRu = val
					} else if vk >= 65 && vk <= 90 {
						charRu = strings.ToUpper(charRu)
					}
				}

				if charEn != "" {
					capsState, _, _ := procGetKeyState.Call(uintptr(VK_CAPITAL))
					isCaps := (capsState & 0x0001) != 0
					EngineAddChar(charEn, charRu, isShift, isCaps)
				} else if !isModifier(vk) {
					EngineClearContext()
				}
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(keyboardHook, uintptr(nCode), wParam, lParam)
	return ret
}
