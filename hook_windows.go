//go:build windows

package main

import (
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx         = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procGetMessage               = user32.NewProc("GetMessageW")
	procPostMessage              = user32.NewProc("PostMessageW")
	procGetForeground            = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	procGetKeyboardLayoutList    = user32.NewProc("GetKeyboardLayoutList")
	procGetKeyState              = user32.NewProc("GetKeyState")
	procSendInput                = user32.NewProc("SendInput")

	keyboardHook uintptr
)

const (
	WH_KEYBOARD_LL            = 13
	WM_KEYDOWN                = 0x0100
	WM_INPUTLANGCHANGEREQUEST = 0x0050
	VK_BACK                   = 0x08
	VK_SPACE                  = 0x20
	VK_RETURN                 = 0x0D
	VK_SHIFT                  = 0x10
	VK_CAPITAL                = 0x14

	INPUT_KEYBOARD    = 1
	KEYEVENTF_KEYUP   = 0x0002
	KEYEVENTF_UNICODE = 0x0004
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

type KeyEvent struct {
	VK    uint32
	Shift bool
	Caps  bool
}

var (
	currentWord             []KeyEvent
	isReplacing             uint32
	lastWord                []KeyEvent
	typedSpaceAfterLastWord bool
	lastPid                 uint32
)

var vkToEn = map[uint32]string{
	48: "0", 49: "1", 50: "2", 51: "3", 52: "4", 53: "5", 54: "6", 55: "7", 56: "8", 57: "9",
	65: "a", 66: "b", 67: "c", 68: "d", 69: "e", 70: "f", 71: "g", 72: "h", 73: "i", 74: "j", 75: "k", 76: "l", 77: "m", 78: "n", 79: "o", 80: "p", 81: "q", 82: "r", 83: "s", 84: "t", 85: "u", 86: "v", 87: "w", 88: "x", 89: "y", 90: "z",
	186: ";", 187: "=", 188: ",", 189: "-", 190: ".", 191: "/", 192: "`", 219: "[", 220: "\\", 221: "]", 222: "'",
}
var vkToRu = map[uint32]string{
	48: "0", 49: "1", 50: "2", 51: "3", 52: "4", 53: "5", 54: "6", 55: "7", 56: "8", 57: "9",
	65: "ф", 66: "и", 67: "с", 68: "в", 69: "у", 70: "а", 71: "п", 72: "р", 73: "ш", 74: "о", 75: "л", 76: "д", 77: "ь", 78: "т", 79: "щ", 80: "з", 81: "й", 82: "к", 83: "ы", 84: "е", 85: "г", 86: "м", 87: "ц", 88: "ч", 89: "н", 90: "я",
	186: "ж", 187: "=", 188: "б", 189: "-", 190: "ю", 191: ".", 192: "ё", 219: "х", 220: "\\", 221: "ъ", 222: "э",
}

func cleanWord(word string) string {
	var res strings.Builder
	for _, r := range word {
		if unicode.IsLetter(r) {
			res.WriteRune(unicode.ToLower(r))
		}
	}
	return res.String()
}

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

// sendUnicodeChar — вставка одного символа через SendInput
func sendUnicodeChar(char uint16, keyUp bool) {
	flags := uint32(KEYEVENTF_UNICODE)
	if keyUp {
		flags |= KEYEVENTF_KEYUP
	}
	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			WVk:     0,
			WScan:   char,
			DwFlags: flags,
		},
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

// sendUnicodeString — вставка строки целиком (с поддержкой surrogate pairs)
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
	// VK_BACK = 0x08
	input := INPUT{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{WVk: VK_BACK, DwFlags: 0}}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	input.Ki.DwFlags = KEYEVENTF_KEYUP
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

// switchLayout — переключение раскладки активного окна через реальный HKL
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

	// wParam=0, lParam=реальный HKL
	procPostMessage.Call(hwnd, WM_INPUTLANGCHANGEREQUEST, 0, nextHkl)
}

func replaceText(deletions int, replacement string, appendChar string, switchLang bool) {
	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(20 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	// Чистим только currentWord — lastWord сохраняем для undo через хоткей
	currentWord = currentWord[:0]

	time.Sleep(15 * time.Millisecond)

	totalDel := deletions
	if appendChar != "" {
		totalDel++
	}
	for i := 0; i < totalDel; i++ {
		simulateBackspace()
		time.Sleep(2 * time.Millisecond)
	}

	time.Sleep(10 * time.Millisecond)
	sendUnicodeString(replacement + appendChar)
	time.Sleep(30 * time.Millisecond)

	if switchLang {
		hwnd, _, _ := procGetForeground.Call()
		if hwnd != 0 {
			switchLayout(hwnd)
		}
	}
}

func getHotkeyVK(hotkeyStr string) uint32 {
	switch strings.ToLower(strings.TrimSpace(hotkeyStr)) {
	case "pause":
		return 0x13
	case "f12":
		return 0x7B
	case "f9":
		return 0x78
	case "f8":
		return 0x77
	case "scrolllock":
		return 0x91
	default:
		return 0x13
	}
}

func processWord(appendChar string, isManual bool) {
	settingsMutex.RLock()
	aiEnabled := settings.EnableAI
	threshold := float64(settings.Threshold) / 100.0
	exceptions := strings.ToLower(settings.Exceptions)
	settingsMutex.RUnlock()

	var wordData []KeyEvent
	var deletions int

	if isManual {
		if len(currentWord) > 0 {
			wordData = make([]KeyEvent, len(currentWord))
			copy(wordData, currentWord)
			deletions = len(wordData)
			currentWord = currentWord[:0]
			lastWord = make([]KeyEvent, len(wordData))
			copy(lastWord, wordData)
			typedSpaceAfterLastWord = false // NEW
		} else if len(lastWord) > 0 {
			wordData = make([]KeyEvent, len(lastWord))
			copy(wordData, lastWord)
			deletions = len(wordData)
			if typedSpaceAfterLastWord {
				deletions++
				appendChar = " "
			}
		} else {
			return
		}
	} else {
		if len(currentWord) == 0 {
			return
		}
		wordData = make([]KeyEvent, len(currentWord))
		copy(wordData, currentWord)
		deletions = len(wordData)

		lastWord = make([]KeyEvent, len(wordData))
		copy(lastWord, wordData)
		typedSpaceAfterLastWord = (appendChar != "")

		currentWord = currentWord[:0]

		if !aiEnabled {
			return
		}
	}

	var enStr, ruStr strings.Builder
	var hasUpper, hasLower bool
	var hasDigit bool

	for i, ev := range wordData {
		charEn := vkToEn[ev.VK]
		charRu := vkToRu[ev.VK]

		if ev.VK >= 48 && ev.VK <= 57 {
			hasDigit = true
		}

		if ev.VK >= 65 && ev.VK <= 90 {
			isUpper := ev.Shift != ev.Caps
			if isUpper {
				charEn = strings.ToUpper(charEn)
				charRu = strings.ToUpper(charRu)
				if i > 0 {
					hasUpper = true
				}
			} else if i > 0 {
				hasLower = true
			}
		}
		enStr.WriteString(charEn)
		ruStr.WriteString(charRu)
	}
	wordEn, wordRu := enStr.String(), ruStr.String()

	wLowerEn, wLowerRu := strings.ToLower(wordEn), strings.ToLower(wordRu)
	cEn := cleanWord(wordEn)
	cRu := cleanWord(wordRu)

	for _, excLine := range strings.Split(exceptions, "\n") {
		excLine = strings.TrimSpace(excLine)
		if excLine == "" {
			continue
		}
		if wLowerEn == excLine || wLowerRu == excLine ||
			cEn == excLine || cRu == excLine {
			return
		}
	}

	// Пароли: блокируем только СМЕШАННЫЙ регистр (iPhone, myPass) и цифры
	if !isManual {
		if hasDigit {
			return
		}
		if hasUpper && hasLower {
			return
		}
	}

	if len(cEn) < 2 {
		return
	}

	hwnd, _, _ := procGetForeground.Call()
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
	isRu := (layoutId & 0xFFFF) == 0x0419

	if isManual {
		if isRu {
			go replaceText(deletions, wordEn, appendChar, false)
		} else {
			go replaceText(deletions, wordRu, appendChar, true)
		}
		return
	}

	if isRu {
		if ShouldConvertWord(cRu, cEn, "ru", threshold) {
			go replaceText(deletions, wordEn, appendChar, false)
		}
	} else {
		if ShouldConvertWord(cEn, cRu, "en", threshold) {
			go replaceText(deletions, wordRu, appendChar, true)
		}
	}
}

func isModifier(vk uint32) bool {
	switch vk {
	case 0x10, 0xA0, 0xA1, 0x11, 0xA2, 0xA3, 0x12, 0xA4, 0xA5, 0x14, 0x5B, 0x5C:
		return true
	}
	return false
}

func hookCallback(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && wParam == WM_KEYDOWN && atomic.LoadUint32(&isReplacing) == 0 {
		kbd := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		vk := kbd.VkCode

		hwnd, _, _ := procGetForeground.Call()
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

		if lastPid != 0 && lastPid != pid {
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
		}
		lastPid = pid

		settingsMutex.RLock()
		targetHotkeyVK := getHotkeyVK(settings.Hotkey)
		settingsMutex.RUnlock()

		if vk == targetHotkeyVK {
			processWord("", true)
		} else if vk == VK_SPACE || vk == VK_RETURN {
			app := " "
			if vk == VK_RETURN {
				app = "\n"
			}
			processWord(app, false)
		} else if vk == VK_BACK {
			if len(currentWord) > 0 {
				currentWord = currentWord[:len(currentWord)-1]
			} else {
				lastWord = lastWord[:0]
			}
		} else if _, ok := vkToEn[vk]; ok {
			if len(currentWord) < 64 {
				shiftState, _, _ := procGetKeyState.Call(uintptr(VK_SHIFT))
				capsState, _, _ := procGetKeyState.Call(uintptr(VK_CAPITAL))
				isShift := (shiftState & 0x8000) != 0
				isCaps := (capsState & 0x0001) != 0
				currentWord = append(currentWord, KeyEvent{VK: vk, Shift: isShift, Caps: isCaps})
			}
		} else if !isModifier(vk) {
			currentWord = currentWord[:0]
		}
	}
	ret, _, _ := procCallNextHookEx.Call(keyboardHook, uintptr(nCode), wParam, lParam)
	return ret
}
