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

type KeyEvent struct {
	VK    uint32
	Shift bool
	Caps  bool
}

type CorrectionData struct {
	OriginalFull  string
	OriginalClean string
	PhysicalEn    string
	Corrected     string
	AppendChar    string
	SwitchedLang  bool
	Timestamp     time.Time
}

var (
	currentWord             []KeyEvent
	isReplacing             uint32
	lastWord                []KeyEvent
	typedSpaceAfterLastWord bool
	lastRenderedWord        string
	lastRenderedSuffix      string
	lastPid                 uint32
	lastProcessName         string
	lastAutoCorrection      *CorrectionData
	backspaceCount          int
	lastShiftTime           time.Time
	shiftReleased           bool   = true
	lastFinalLang           string = "en"
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

var fullEnToRu = map[rune]rune{
	'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш', 'o': 'щ', 'p': 'з', '[': 'х', ']': 'ъ',
	'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л', 'l': 'д', ';': 'ж', '\'': 'э',
	'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь', ',': 'б', '.': 'ю', '`': 'ё',
	'Q': 'Й', 'W': 'Ц', 'E': 'У', 'R': 'К', 'T': 'Е', 'Y': 'Н', 'U': 'Г', 'I': 'Ш', 'O': 'Щ', 'P': 'З', '{': 'Х', '}': 'Ъ',
	'A': 'Ф', 'S': 'Ы', 'D': 'В', 'F': 'А', 'G': 'П', 'H': 'Р', 'J': 'О', 'K': 'Л', 'L': 'Д', ':': 'Ж', '"': 'Э',
	'Z': 'Я', 'X': 'Ч', 'C': 'С', 'V': 'М', 'B': 'И', 'N': 'Т', 'M': 'Ь', '<': 'Б', '>': 'Ю', '~': 'Ё',
	'@': '"', '#': '№', '$': ';', '^': ':', '&': '?', '/': '.', '?': ',', '|': '/',
}

var fullRuToEn = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`',
	'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
	'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J', 'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
	'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M', 'Б': '<', 'Ю': '>', 'Ё': '~',
	'"': '@', '№': '#', ';': '$', ':': '^', '?': '&', ',': '?',
}

func convertSelectedLayout(text string) (string, bool) {
	ruCount, enCount := 0, 0
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			ruCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			enCount++
		}
	}

	toEn := ruCount >= enCount
	var res strings.Builder
	for _, r := range text {
		if toEn {
			if mapped, ok := fullRuToEn[r]; ok {
				res.WriteRune(mapped)
			} else if r == '.' {
				res.WriteRune('/')
			} else {
				res.WriteRune(r)
			}
		} else {
			if mapped, ok := fullEnToRu[r]; ok {
				res.WriteRune(mapped)
			} else {
				res.WriteRune(r)
			}
		}
	}
	return res.String(), toEn
}

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

func clearClipboard() {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret != 0 {
		procEmptyClipboard.Call()
		procCloseClipboard.Call()
	}
}

func sendKeyCombo(vk uint16, keyUp bool) {
	flags := uint32(0)
	if keyUp {
		flags = KEYEVENTF_KEYUP
	}
	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			WVk:     vk,
			DwFlags: flags,
		},
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func releaseModifiers() {
	sendKeyCombo(VK_SHIFT, true)
	sendKeyCombo(VK_LSHIFT, true)
	sendKeyCombo(VK_RSHIFT, true)
	sendKeyCombo(VK_MENU, true)
	sendKeyCombo(VK_LMENU, true)
	sendKeyCombo(VK_RMENU, true)
}

func sendCtrlKey(vk uint16) {
	releaseModifiers()
	time.Sleep(10 * time.Millisecond)
	sendKeyCombo(VK_CONTROL, false)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(vk, false)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(vk, true)
	time.Sleep(5 * time.Millisecond)
	sendKeyCombo(VK_CONTROL, true)
}

func tryTransformSelection(mode string) bool {
	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(30 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	backupText, hadBackup := getClipboardText()
	clearClipboard()

	sendCtrlKey(0x43) // Ctrl + C
	time.Sleep(65 * time.Millisecond)

	selected, ok := getClipboardText()
	if !ok || selected == "" {
		if hadBackup {
			setClipboardText(backupText)
		}
		return false
	}

	var transformed string
	var switchedToEn bool
	if mode == "layout" {
		transformed, switchedToEn = convertSelectedLayout(selected)
	} else {
		transformed = cycleCase(selected)
	}

	if transformed == selected {
		if hadBackup {
			setClipboardText(backupText)
		}
		return true
	}

	setClipboardText(transformed)
	time.Sleep(15 * time.Millisecond)
	sendCtrlKey(0x56) // Ctrl + V
	time.Sleep(65 * time.Millisecond)

	if mode == "layout" {
		hwnd, _, _ := procGetForeground.Call()
		if hwnd != 0 {
			threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
			layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
			isRu := (layoutId & 0xFFFF) == 0x0419
			if (isRu && switchedToEn) || (!isRu && !switchedToEn) {
				switchLayout(hwnd)
			}
		}
	}

	if hadBackup {
		setClipboardText(backupText)
	} else {
		clearClipboard()
	}

	currentWord = currentWord[:0]
	lastWord = lastWord[:0]
	lastRenderedWord = ""
	return true
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

func isBoundaryEn(charEn string) bool {
	return strings.ContainsAny(charEn, "\"'()[]{}<>=:;,!?%^&*|~+#")
}

func isITPattern(wordEn string) bool {
	if strings.Contains(wordEn, "_") {
		return true
	}
	if strings.Contains(wordEn, "-") {
		parts := strings.Split(wordEn, "-")
		if len(parts) > 1 && parts[0] != "" && parts[len(parts)-1] != "" {
			return true
		}
	}
	hasDigit, hasDot := false, false
	for _, r := range wordEn {
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == '.' {
			hasDot = true
		}
	}
	return hasDigit && hasDot
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

func cleanWord(word string) string {
	var res strings.Builder
	for _, r := range word {
		if unicode.IsLetter(r) {
			res.WriteRune(unicode.ToLower(r))
		}
	}
	return res.String()
}

func fixInvertedCaps(word string) string {
	runes := []rune(word)
	if len(runes) < 2 {
		return word
	}
	if unicode.IsLower(runes[0]) {
		allRestUpper := true
		hasLetters := false
		for i := 1; i < len(runes); i++ {
			if unicode.IsLetter(runes[i]) {
				hasLetters = true
				if !unicode.IsUpper(runes[i]) {
					allRestUpper = false
					break
				}
			}
		}
		if hasLetters && allRestUpper {
			res := make([]rune, len(runes))
			res[0] = unicode.ToUpper(runes[0])
			for i := 1; i < len(runes); i++ {
				res[i] = unicode.ToLower(runes[i])
			}
			return string(res)
		}
	}
	return word
}

func cycleCase(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}

	isAllLower := true
	isAllUpper := true
	hasLetters := false

	for _, r := range runes {
		if unicode.IsLetter(r) {
			hasLetters = true
			if !unicode.IsLower(r) {
				isAllLower = false
			}
			if !unicode.IsUpper(r) {
				isAllUpper = false
			}
		}
	}

	if !hasLetters {
		return text
	}

	if isAllLower {
		return strings.ToUpper(text)
	}
	if isAllUpper {
		res := []rune(strings.ToLower(text))
		capitalizeNext := true
		for i, r := range res {
			if unicode.IsLetter(r) {
				if capitalizeNext {
					res[i] = unicode.ToUpper(r)
					capitalizeNext = false
				}
			} else if unicode.IsSpace(r) || strings.ContainsRune("-_\"'([{", r) {
				capitalizeNext = true
			}
		}
		return string(res)
	}
	return strings.ToLower(text)
}

func buildWordStrings(events []KeyEvent) (wordEn string, wordRu string, hasUpper bool, hasLower bool, hasDigit bool, hasCapsLock bool) {
	var enStr, ruStr strings.Builder
	for i, ev := range events {
		charEn := vkToEn[ev.VK]
		charRu := vkToRu[ev.VK]
		if ev.Caps {
			hasCapsLock = true
		}
		if ev.Shift {
			if val, ok := vkToEnShift[ev.VK]; ok {
				charEn = val
			}
			if val, ok := vkToRuShift[ev.VK]; ok {
				charRu = val
			}
		}
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
		} else {
			switch ev.VK {
			case 186, 188, 190, 192, 219, 221, 222:
				if ev.Caps {
					if ev.Shift {
						charRu = strings.ToLower(charRu)
					} else {
						charRu = strings.ToUpper(charRu)
					}
				}
			}
		}
		enStr.WriteString(charEn)
		ruStr.WriteString(charRu)
	}
	return enStr.String(), ruStr.String(), hasUpper, hasLower, hasDigit, hasCapsLock
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

func replaceText(deletions int, replacement string, appendChar string, switchLang bool) {
	lastRenderedWord = replacement
	lastRenderedSuffix = appendChar

	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(20 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	time.Sleep(15 * time.Millisecond)
	totalDel := deletions
	if appendChar != "" {
		totalDel += len([]rune(appendChar))
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

func handleCaseToggle() {
	if tryTransformSelection("case") {
		return
	}

	hwnd, _, _ := procGetForeground.Call()
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
	isRu := (layoutId & 0xFFFF) == 0x0419

	if len(currentWord) > 0 {
		wordEn, wordRu, _, _, _, _ := buildWordStrings(currentWord)
		target := wordEn
		if isRu {
			target = wordRu
		}
		deletions := len([]rune(target))
		cycled := cycleCase(target)

		lastWord = make([]KeyEvent, len(currentWord))
		copy(lastWord, currentWord)
		typedSpaceAfterLastWord = false
		currentWord = currentWord[:0]

		replaceText(deletions, cycled, "", false)
		return
	}

	if lastRenderedWord != "" {
		deletions := len([]rune(lastRenderedWord))
		cycled := cycleCase(lastRenderedWord)
		replaceText(deletions, cycled, lastRenderedSuffix, false)
	}
}

func handleManualLayoutSwitch() {
	if tryTransformSelection("layout") {
		return
	}
	processWord("", true)
}

func learnException(word string) {
	word = strings.TrimSpace(strings.ToLower(word))
	if len(word) < 2 {
		return
	}
	if currentModel != nil && currentModel.KnownWords != nil {
		currentModel.KnownWords[word] = true
	}
	settingsMutex.RLock()
	existing := settings.Exceptions
	newSettings := settings
	settingsMutex.RUnlock()

	for _, e := range strings.Split(existing, "\n") {
		if strings.TrimSpace(e) == word {
			return
		}
	}
	if newSettings.Exceptions != "" && !strings.HasSuffix(newSettings.Exceptions, "\n") {
		newSettings.Exceptions += "\n"
	}
	newSettings.Exceptions += word
	go saveSettings(newSettings)
}

func learnForceConvert(word string) {
	word = strings.TrimSpace(strings.ToLower(word))
	if len(word) < 2 {
		return
	}
	settingsMutex.RLock()
	existing := settings.ForceConvert
	newSettings := settings
	settingsMutex.RUnlock()

	for _, e := range strings.Split(existing, "\n") {
		if strings.TrimSpace(e) == word {
			return
		}
	}
	if newSettings.ForceConvert != "" && !strings.HasSuffix(newSettings.ForceConvert, "\n") {
		newSettings.ForceConvert += "\n"
	}
	newSettings.ForceConvert += word
	go saveSettings(newSettings)
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

func processLiveTypo() {
	settingsMutex.RLock()
	aiEnabled := settings.EnableAI
	exceptions := strings.ToLower(settings.Exceptions)
	blockFlags := settings.BlockFlags
	blockPaths := settings.BlockPaths
	settingsMutex.RUnlock()

	if !aiEnabled || len(currentWord) < 3 {
		return
	}

	wordEn, wordRu, hasUpper, hasLower, hasDigit, hasCapsLock := buildWordStrings(currentWord)
	wLowerEn, wLowerRu := strings.ToLower(wordEn), strings.ToLower(wordRu)
	cEn, cRu := cleanWord(wordEn), cleanWord(wordRu)

	for _, excLine := range strings.Split(exceptions, "\n") {
		excLine = strings.TrimSpace(excLine)
		if excLine == "" {
			continue
		}
		if wLowerEn == excLine || wLowerRu == excLine || cEn == excLine || cRu == excLine {
			return
		}
	}

	hwnd, _, _ := procGetForeground.Call()
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
	isRu := (layoutId & 0xFFFF) == 0x0419
	deletions := len(currentWord)

	if isITPattern(wordEn) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
			return
		} else {
			return
		}
	}

	if hasDigit || (hasUpper && hasLower && !hasCapsLock) {
		return
	}

	firstVK := currentWord[0].VK
	isFlag := firstVK == 189
	isPath := firstVK == 220 || firstVK == 191

	if (isFlag && blockFlags) || (isPath && blockPaths) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
			return
		} else {
			return
		}
	}

	if isRu {
		if IsHardTypo(cRu, true, false) && !IsHardTypo(cEn, false, false) {
			finalEn := wordEn
			if hasCapsLock {
				finalEn = fixInvertedCaps(finalEn)
			}
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordRu,
				OriginalClean: cRu,
				PhysicalEn:    wLowerEn,
				Corrected:     finalEn,
				AppendChar:    "",
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
			lastFinalLang = "en"
			go replaceText(deletions, finalEn, "", true)
		}
	} else {
		if IsHardTypo(cEn, false, false) && !IsHardTypo(cRu, true, false) {
			finalRu := wordRu
			if hasCapsLock {
				finalRu = fixInvertedCaps(finalRu)
			}
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordEn,
				OriginalClean: cEn,
				PhysicalEn:    wLowerEn,
				Corrected:     finalRu,
				AppendChar:    "",
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
			lastFinalLang = "ru"
			go replaceText(deletions, finalRu, "", true)
		}
	}
}

func processWord(appendChar string, isManual bool) {
	settingsMutex.RLock()
	aiEnabled := settings.EnableAI
	threshold := float64(settings.Threshold) / 100.0
	exceptions := strings.ToLower(settings.Exceptions)
	forceConvert := strings.ToLower(settings.ForceConvert)
	blockPaths := settings.BlockPaths
	blockFlags := settings.BlockFlags
	useMarkov := settings.UseMarkov
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
			typedSpaceAfterLastWord = false
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

	wordEn, wordRu, hasUpper, hasLower, hasDigit, hasCapsLock := buildWordStrings(wordData)
	wLowerEn, wLowerRu := strings.ToLower(wordEn), strings.ToLower(wordRu)
	cEn := cleanWord(wordEn)
	cRu := cleanWord(wordRu)

	hwnd, _, _ := procGetForeground.Call()
	threadId, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layoutId, _, _ := procGetKeyboardLayout.Call(threadId)
	isRu := (layoutId & 0xFFFF) == 0x0419

	if isRu {
		lastRenderedWord = wordRu
	} else {
		lastRenderedWord = wordEn
	}
	lastRenderedSuffix = appendChar

	for _, excLine := range strings.Split(exceptions, "\n") {
		excLine = strings.TrimSpace(excLine)
		if excLine == "" {
			continue
		}
		if wLowerEn == excLine || wLowerRu == excLine || cEn == excLine || cRu == excLine {
			return
		}
	}

	if isManual {
		if isRu {
			learnForceConvert(cRu)
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, appendChar, true)
		} else {
			learnForceConvert(cEn)
			lastFinalLang = "ru"
			go replaceText(deletions, wordRu, appendChar, true)
		}
		return
	}

	for _, fcLine := range strings.Split(forceConvert, "\n") {
		fcLine = strings.TrimSpace(fcLine)
		if fcLine == "" {
			continue
		}
		if cEn == fcLine && !isRu {
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordEn,
				OriginalClean: cEn,
				PhysicalEn:    wLowerEn,
				Corrected:     wordRu,
				AppendChar:    appendChar,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			lastFinalLang = "ru"
			go replaceText(deletions, wordRu, appendChar, true)
			return
		}
		if cRu == fcLine && isRu {
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordRu,
				OriginalClean: cRu,
				PhysicalEn:    wLowerEn,
				Corrected:     wordEn,
				AppendChar:    appendChar,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, appendChar, true)
			return
		}
	}

	if isITPattern(wordEn) {
		if isRu {
			lastFinalLang = "en"
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordRu,
				OriginalClean: cRu,
				PhysicalEn:    wLowerEn,
				Corrected:     wordEn,
				AppendChar:    appendChar,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			backspaceCount = 0
			go replaceText(deletions, wordEn, appendChar, true)
			return
		} else {
			lastFinalLang = "en"
			return
		}
	}

	if !isManual {
		if hasDigit {
			return
		}
		if hasUpper && hasLower && !hasCapsLock {
			return
		}
	}

	if len(cEn) < 2 {
		return
	}

	if len(wordData) > 0 {
		firstVK := wordData[0].VK
		isFlag := firstVK == 189
		isPath := firstVK == 220 || firstVK == 191
		if (isFlag && blockFlags) || (isPath && blockPaths) {
			if isRu {
				lastFinalLang = "en"
				lastAutoCorrection = &CorrectionData{
					OriginalFull:  wordRu,
					OriginalClean: cRu,
					PhysicalEn:    wLowerEn,
					Corrected:     wordEn,
					AppendChar:    appendChar,
					SwitchedLang:  true,
					Timestamp:     time.Now(),
				}
				incrementLayoutCorrection(len([]rune(cRu)))
				backspaceCount = 0
				go replaceText(deletions, wordEn, appendChar, true)
				return
			} else {
				lastFinalLang = "en"
				return
			}
		}
	}

	adjustedThreshold := threshold
	if useMarkov {
		if isRu && lastFinalLang == "en" {
			adjustedThreshold -= 0.15
		} else if !isRu && lastFinalLang == "ru" {
			adjustedThreshold -= 0.15
		}
	}

	if len(cEn) >= 3 {
		dist := CalcAverageJump(wLowerEn)
		visualWord := cEn
		if isRu {
			visualWord = cRu
		}
		addTopoRecord(visualWord, dist)
	}

	if isRu {
		if ShouldConvertWord(cRu, cEn, "ru", adjustedThreshold) {
			finalEn := wordEn
			if hasCapsLock {
				finalEn = fixInvertedCaps(finalEn)
			}
			correctedEn, _ := ApplyAutoCorrect(finalEn, "en")
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordRu,
				OriginalClean: cRu,
				PhysicalEn:    wLowerEn,
				Corrected:     correctedEn,
				AppendChar:    appendChar,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			lastFinalLang = "en"
			go replaceText(deletions, correctedEn, appendChar, true)
		} else {
			lastFinalLang = "ru"
			if correctedRu, changed := ApplyAutoCorrect(wordRu, "ru"); changed {
				lastAutoCorrection = &CorrectionData{
					OriginalFull:  wordRu,
					OriginalClean: cRu,
					PhysicalEn:    wLowerEn,
					Corrected:     correctedRu,
					AppendChar:    appendChar,
					SwitchedLang:  false,
					Timestamp:     time.Now(),
				}
				incrementTypoCorrection(len([]rune(cRu)))
				backspaceCount = 0
				go replaceText(deletions, correctedRu, appendChar, false)
			}
		}
	} else {
		if ShouldConvertWord(cEn, cRu, "en", adjustedThreshold) {
			finalRu := wordRu
			if hasCapsLock {
				finalRu = fixInvertedCaps(finalRu)
			}
			correctedRu, _ := ApplyAutoCorrect(finalRu, "ru")
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordEn,
				OriginalClean: cEn,
				PhysicalEn:    wLowerEn,
				Corrected:     correctedRu,
				AppendChar:    appendChar,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			backspaceCount = 0
			lastFinalLang = "ru"
			go replaceText(deletions, correctedRu, appendChar, true)
		} else {
			lastFinalLang = "en"
			if correctedEn, changed := ApplyAutoCorrect(wordEn, "en"); changed {
				lastAutoCorrection = &CorrectionData{
					OriginalFull:  wordEn,
					OriginalClean: cEn,
					PhysicalEn:    wLowerEn,
					Corrected:     correctedEn,
					AppendChar:    appendChar,
					SwitchedLang:  false,
					Timestamp:     time.Now(),
				}
				incrementTypoCorrection(len([]rune(cEn)))
				backspaceCount = 0
				go replaceText(deletions, correctedEn, appendChar, false)
			}
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
					currentWord = currentWord[:0]
					lastWord = lastWord[:0]
					lastRenderedWord = ""
				}
				lastProcessName = getProcessName(pid)
				lastPid = pid
			}

			settingsMutex.RLock()
			blacklist := strings.ToLower(settings.Blacklist)
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
					now := time.Now()
					if now.Sub(lastShiftTime) < 400*time.Millisecond {
						if lastAutoCorrection != nil && time.Since(lastAutoCorrection.Timestamp) < 15*time.Second {
							deletions := len([]rune(lastAutoCorrection.Corrected))
							go replaceText(deletions, lastAutoCorrection.OriginalFull, lastAutoCorrection.AppendChar, lastAutoCorrection.SwitchedLang)
							learnException(lastAutoCorrection.OriginalClean)

							if lastAutoCorrection.SwitchedLang {
								revertHeatmapError(lastAutoCorrection.PhysicalEn)
							}
							incrementUndoCount(len([]rune(lastAutoCorrection.OriginalClean)), lastAutoCorrection.SwitchedLang)
							lastAutoCorrection = nil
						}
						lastShiftTime = time.Time{}
					} else {
						lastShiftTime = now
					}
					shiftReleased = false
				}
			} else if vk != VK_BACK {
				lastShiftTime = time.Time{}
				backspaceCount = 0
			}

			if vk == VK_BACK {
				if lastAutoCorrection != nil && time.Since(lastAutoCorrection.Timestamp) < 15*time.Second {
					backspaceCount++
					targetLen := len([]rune(lastAutoCorrection.Corrected)) + len([]rune(lastAutoCorrection.AppendChar))
					if backspaceCount >= targetLen {
						learnException(lastAutoCorrection.OriginalClean)
						if lastAutoCorrection.SwitchedLang {
							revertHeatmapError(lastAutoCorrection.PhysicalEn)
						}
						incrementUndoCount(len([]rune(lastAutoCorrection.OriginalClean)), lastAutoCorrection.SwitchedLang)
						lastAutoCorrection = nil
						backspaceCount = 0
					}
				} else {
					backspaceCount = 0
				}
			}

			settingsMutex.RLock()
			targetHotkeyVK, targetNeedShift, targetNeedAlt := parseHotkey(settings.Hotkey)
			caseHotkeyVK, caseNeedShift, caseNeedAlt := parseHotkey(settings.CaseHotkey)
			liveTypoEnabled := settings.LiveTypoEnabled
			liveTypoChar := settings.LiveTypoChar
			settingsMutex.RUnlock()

			if liveTypoChar < 3 {
				liveTypoChar = 4
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

			charEn := getCharEn(vk, isShift)

			if vk == caseHotkeyVK && isShift == caseNeedShift && isAlt == caseNeedAlt {
				go handleCaseToggle()
				return 1
			}

			if vk == targetHotkeyVK && isShift == targetNeedShift && isAlt == targetNeedAlt {
				go handleManualLayoutSwitch()
				return 1
			}

			if vk == VK_SPACE {
				processWord(" ", false)
			} else if vk == VK_RETURN {
				processWord("\n", false)
			} else if vk == VK_TAB {
				processWord("\t", false)
			} else if vk == VK_BACK {
				if len(currentWord) > 0 {
					currentWord = currentWord[:len(currentWord)-1]
				} else {
					lastWord = lastWord[:0]
					lastRenderedWord = ""
				}
			} else if charEn != "" && isBoundaryEn(charEn) {
				processWord(charEn, false)
			} else if charEn != "" {
				capsState, _, _ := procGetKeyState.Call(uintptr(VK_CAPITAL))
				isCaps := (capsState & 0x0001) != 0
				currentWord = append(currentWord, KeyEvent{VK: vk, Shift: isShift, Caps: isCaps})

				if liveTypoEnabled && len(currentWord) >= liveTypoChar {
					processLiveTypo()
				}
			} else if !isModifier(vk) {
				currentWord = currentWord[:0]
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(keyboardHook, uintptr(nCode), wParam, lParam)
	return ret
}
