//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	EV_SYN     = 0x00
	EV_KEY     = 0x01
	EV_LED     = 0x11
	SYN_REPORT = 0x00
	LED_CAPSL  = 0x01

	KEY_ESC        = 1
	KEY_1          = 2
	KEY_2          = 3
	KEY_3          = 4
	KEY_4          = 5
	KEY_5          = 6
	KEY_6          = 7
	KEY_7          = 8
	KEY_8          = 9
	KEY_9          = 10
	KEY_0          = 11
	KEY_MINUS      = 12
	KEY_EQUAL      = 13
	KEY_BACKSPACE  = 14
	KEY_TAB        = 15
	KEY_Q          = 16
	KEY_W          = 17
	KEY_E          = 18
	KEY_R          = 19
	KEY_T          = 20
	KEY_Y          = 21
	KEY_U          = 22
	KEY_I          = 23
	KEY_O          = 24
	KEY_P          = 25
	KEY_LEFTBRACE  = 26
	KEY_RIGHTBRACE = 27
	KEY_ENTER      = 28
	KEY_LEFTCTRL   = 29
	KEY_A          = 30
	KEY_S          = 31
	KEY_D          = 32
	KEY_F          = 33
	KEY_G          = 34
	KEY_H          = 35
	KEY_J          = 36
	KEY_K          = 37
	KEY_L          = 38
	KEY_SEMICOLON  = 39
	KEY_APOSTROPHE = 40
	KEY_GRAVE      = 41
	KEY_LEFTSHIFT  = 42
	KEY_BACKSLASH  = 43
	KEY_Z          = 44
	KEY_X          = 45
	KEY_C          = 46
	KEY_V          = 47
	KEY_B          = 48
	KEY_N          = 49
	KEY_M          = 50
	KEY_COMMA      = 51
	KEY_DOT        = 52
	KEY_SLASH      = 53
	KEY_RIGHTSHIFT = 54
	KEY_LEFTALT    = 56
	KEY_SPACE      = 57
	KEY_CAPSLOCK   = 58
	KEY_F8         = 66
	KEY_F9         = 67
	KEY_SCROLLLOCK = 70
	KEY_F12        = 88
	KEY_RIGHTCTRL  = 97
	KEY_RIGHTALT   = 100
	KEY_PAUSE      = 119
	KEY_LEFTMETA   = 125
	KEY_RIGHTMETA  = 126

	EVIOCGNAME_256 = 0x81004506
	EVIOCGBIT_EV   = 0x80084520
	EVIOCGBIT_KEY  = 0x80604521
	EVIOCGLED      = 0x80084519
	UI_SET_EVBIT   = 0x40045564
	UI_SET_KEYBIT  = 0x40045565
	UI_DEV_SETUP   = 0x405c5503
	UI_DEV_CREATE  = 0x5501
	UI_DEV_DESTROY = 0x5502
	BUS_USB        = 0x03
)

type linuxInputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

type uinputSetup struct {
	ID struct {
		Bustype, Vendor, Product, Version uint16
	}
	Name         [80]byte
	FfEffectsMax uint32
}

var (
	uinputFd             int = -1
	cachedActiveProc     string
	lastActiveProc       string
	lastProcCheckTime    time.Time
	hasKdotool           bool
	hasXdotool           bool
	qdbusBinary          string
	waylandToolsDetected bool

	lShiftDown, rShiftDown bool
	lCtrlDown, rCtrlDown   bool
	lAltDown, rAltDown     bool
	lMetaDown, rMetaDown   bool
	capsLockState          bool
	shiftReleased          bool = true

	// Механизм graceful shutdown для горутин хука
	hookQuit chan struct{}
	hookKeys int32 // атомарный счётчик активных горутин хука
)

var evToEn = map[uint16]string{
	KEY_0: "0", KEY_1: "1", KEY_2: "2", KEY_3: "3", KEY_4: "4", KEY_5: "5", KEY_6: "6", KEY_7: "7", KEY_8: "8", KEY_9: "9",
	KEY_A: "a", KEY_B: "b", KEY_C: "c", KEY_D: "d", KEY_E: "e", KEY_F: "f", KEY_G: "g", KEY_H: "h", KEY_I: "i", KEY_J: "j", KEY_K: "k", KEY_L: "l", KEY_M: "m", KEY_N: "n", KEY_O: "o", KEY_P: "p", KEY_Q: "q", KEY_R: "r", KEY_S: "s", KEY_T: "t", KEY_U: "u", KEY_V: "v", KEY_W: "w", KEY_X: "x", KEY_Y: "y", KEY_Z: "z",
	KEY_SEMICOLON: ";", KEY_EQUAL: "=", KEY_COMMA: ",", KEY_MINUS: "-", KEY_DOT: ".", KEY_SLASH: "/", KEY_GRAVE: "`", KEY_LEFTBRACE: "[", KEY_BACKSLASH: `\`, KEY_RIGHTBRACE: "]", KEY_APOSTROPHE: "'",
}
var evToRu = map[uint16]string{
	KEY_0: "0", KEY_1: "1", KEY_2: "2", KEY_3: "3", KEY_4: "4", KEY_5: "5", KEY_6: "6", KEY_7: "7", KEY_8: "8", KEY_9: "9",
	KEY_A: "ф", KEY_B: "и", KEY_C: "с", KEY_D: "в", KEY_E: "у", KEY_F: "а", KEY_G: "п", KEY_H: "р", KEY_I: "ш", KEY_J: "о", KEY_K: "л", KEY_L: "д", KEY_M: "ь", KEY_N: "т", KEY_O: "щ", KEY_P: "з", KEY_Q: "й", KEY_R: "к", KEY_S: "ы", KEY_T: "е", KEY_U: "г", KEY_V: "м", KEY_W: "ц", KEY_X: "ч", KEY_Y: "н", KEY_Z: "я",
	KEY_SEMICOLON: "ж", KEY_EQUAL: "=", KEY_COMMA: "б", KEY_MINUS: "-", KEY_DOT: "ю", KEY_SLASH: ".", KEY_GRAVE: "ё", KEY_LEFTBRACE: "х", KEY_BACKSLASH: `\`, KEY_RIGHTBRACE: "ъ", KEY_APOSTROPHE: "э",
}
var evToEnShift = map[uint16]string{
	KEY_0: ")", KEY_1: "!", KEY_2: "@", KEY_3: "#", KEY_4: "$", KEY_5: "%", KEY_6: "^", KEY_7: "&", KEY_8: "*", KEY_9: "(",
	KEY_SEMICOLON: ":", KEY_EQUAL: "+", KEY_COMMA: "<", KEY_MINUS: "_", KEY_DOT: ">", KEY_SLASH: "?", KEY_GRAVE: "~", KEY_LEFTBRACE: "{", KEY_BACKSLASH: "|", KEY_RIGHTBRACE: "}", KEY_APOSTROPHE: `"`,
}
var evToRuShift = map[uint16]string{
	KEY_0: ")", KEY_1: "!", KEY_2: `"`, KEY_3: "№", KEY_4: ";", KEY_5: "%", KEY_6: ":", KEY_7: "?", KEY_8: "*", KEY_9: "(",
	KEY_SEMICOLON: "Ж", KEY_EQUAL: "+", KEY_COMMA: "Б", KEY_MINUS: "_", KEY_DOT: "Ю", KEY_SLASH: ",", KEY_GRAVE: "Ё", KEY_LEFTBRACE: "Х", KEY_BACKSLASH: "/", KEY_RIGHTBRACE: "Ъ", KEY_APOSTROPHE: "Э",
}

var hexToKey = map[rune]uint16{
	'0': KEY_0, '1': KEY_1, '2': KEY_2, '3': KEY_3, '4': KEY_4, '5': KEY_5, '6': KEY_6, '7': KEY_7, '8': KEY_8, '9': KEY_9,
	'a': KEY_A, 'b': KEY_B, 'c': KEY_C, 'd': KEY_D, 'e': KEY_E, 'f': KEY_F,
}

// --- РЕАЛИЗАЦИЯ ИНТЕРФЕЙСОВ ДЛЯ engine.go ---

func osSimulateBackspace() {
	simulateBackspace()
}

func osSendString(text string) {
	sendUnicodeString(text)
}

func osSwitchLayout() {
	switchLayoutLinux()
}

func osIsRussianLayout() bool {
	return isRussianLayoutLinux()
}

func osGetClipboard() (string, bool) {
	return getClipboardLinux()
}

func osSetClipboard(text string) bool {
	setClipboardLinux(text)
	return true
}

func osSendCtrlKey(key string) {
	if key == "c" {
		sendCtrlKeyLinux(KEY_C)
	} else if key == "v" {
		sendCtrlKeyLinux(KEY_V)
	}
}

// --- СИСТЕМНЫЕ ФУНКЦИИ LINUX ---

func detectDesktopTools() {
	if waylandToolsDetected {
		return
	}
	waylandToolsDetected = true
	if _, err := exec.LookPath("kdotool"); err == nil {
		hasKdotool = true
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		hasXdotool = true
	}
	if _, err := exec.LookPath("qdbus6"); err == nil {
		qdbusBinary = "qdbus6"
	} else if _, err := exec.LookPath("qdbus"); err == nil {
		qdbusBinary = "qdbus"
	}
}

func readProcessNameByPID(pidStr string) string {
	pidStr = strings.TrimSpace(pidStr)
	if pidStr == "" || pidStr == "0" {
		return ""
	}
	if comm, err := os.ReadFile("/proc/" + pidStr + "/comm"); err == nil {
		return strings.ToLower(strings.TrimSpace(string(comm)))
	}
	return ""
}

func getActiveProcessLinux() string {
	if time.Since(lastProcCheckTime) < 250*time.Millisecond {
		return cachedActiveProc
	}
	lastProcCheckTime = time.Now()
	detectDesktopTools()

	var detected string
	if hasKdotool {
		if out, err := exec.Command("kdotool", "getactivewindow", "getwindowpid").Output(); err == nil {
			detected = readProcessNameByPID(string(out))
		}
		if detected == "" {
			if out, err := exec.Command("kdotool", "getactivewindow", "getwindowclassname").Output(); err == nil {
				detected = strings.ToLower(strings.TrimSpace(string(out)))
			}
		}
	}
	if detected == "" && qdbusBinary != "" && os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		detected = queryKWinActiveWindowViaDBus()
	}
	if detected == "" && hasXdotool {
		if out, err := exec.Command("xdotool", "getactivewindow", "getwindowpid").Output(); err == nil {
			detected = readProcessNameByPID(string(out))
		}
	}
	cachedActiveProc = detected
	return detected
}

func queryKWinActiveWindowViaDBus() string {
	tmpFile, err := os.CreateTemp("", "goswitcher_kwin_*.js")
	if err != nil {
		return ""
	}
	scriptPath := tmpFile.Name()
	defer os.Remove(scriptPath)

	tmpFile.WriteString(`var w=workspace.activeWindow||workspace.activeClient;if(w){print("GOSWITCHER_WIN:"+w.pid+":"+w.resourceClass);}`)
	tmpFile.Close()

	out, err := exec.Command(qdbusBinary, "org.kde.KWin", "/Scripting", "org.kde.kwin.Scripting.loadScript", scriptPath, "goswitcher_probe").Output()
	if err != nil {
		return ""
	}
	scriptId := strings.TrimSpace(string(out))
	if scriptId == "" {
		return ""
	}

	if err := exec.Command(qdbusBinary, "org.kde.KWin", "/Scripting/Script"+scriptId, "org.kde.kwin.Script.run").Run(); err != nil {
		exec.Command(qdbusBinary, "org.kde.KWin", "/"+scriptId, "org.kde.kwin.Script.run").Run()
	}
	exec.Command(qdbusBinary, "org.kde.KWin", "/Scripting", "org.kde.kwin.Scripting.unloadScript", "goswitcher_probe").Run()

	if jOut, err := exec.Command("journalctl", "--user", "-u", "plasma-kwin_wayland.service", "-n", "5", "--no-pager", "-o", "cat").Output(); err == nil {
		lines := strings.Split(string(jOut), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if idx := strings.Index(lines[i], "GOSWITCHER_WIN:"); idx != -1 {
				parts := strings.Split(strings.TrimSpace(lines[i][idx:]), ":")
				if len(parts) >= 2 {
					if procName := readProcessNameByPID(parts[1]); procName != "" {
						return procName
					}
				}
				if len(parts) >= 3 && parts[2] != "" {
					return strings.ToLower(parts[2])
				}
			}
		}
	}
	return ""
}

func initUinput() error {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	uinputFd = fd

	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_KEY)
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_SYN)
	for code := uint16(1); code < 248; code++ {
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_KEYBIT, uintptr(code))
	}

	var setup uinputSetup
	setup.ID.Bustype = BUS_USB
	setup.ID.Vendor = 0x1234
	setup.ID.Product = 0x5678
	setup.ID.Version = 1
	copy(setup.Name[:], "GoSwitcher Virtual Keyboard")
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_SETUP, uintptr(unsafe.Pointer(&setup)))
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_CREATE, 0)
	time.Sleep(150 * time.Millisecond)
	return nil
}

func emitKey(code uint16, value int32) {
	if uinputFd < 0 {
		return
	}
	ev := linuxInputEvent{Type: EV_KEY, Code: code, Value: value}
	syn := linuxInputEvent{Type: EV_SYN, Code: SYN_REPORT, Value: 0}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, ev)
	binary.Write(&buf, binary.LittleEndian, syn)
	syscall.Write(uinputFd, buf.Bytes())
}

func tapKey(code uint16) {
	emitKey(code, 1)
	time.Sleep(2 * time.Millisecond)
	emitKey(code, 0)
	time.Sleep(2 * time.Millisecond)
}

func simulateBackspace() { tapKey(KEY_BACKSPACE) }

func sendUnicodeRune(r rune) {
	if r == '\n' {
		tapKey(KEY_ENTER)
		return
	}
	if r == '\t' {
		tapKey(KEY_TAB)
		return
	}
	if r == ' ' {
		tapKey(KEY_SPACE)
		return
	}

	hexStr := fmt.Sprintf("%x", r)
	emitKey(KEY_LEFTCTRL, 1)
	emitKey(KEY_LEFTSHIFT, 1)
	time.Sleep(2 * time.Millisecond)
	tapKey(KEY_U)
	emitKey(KEY_LEFTSHIFT, 0)
	emitKey(KEY_LEFTCTRL, 0)
	time.Sleep(3 * time.Millisecond)

	for _, h := range hexStr {
		if k, ok := hexToKey[h]; ok {
			tapKey(k)
		}
	}
	tapKey(KEY_ENTER)
	time.Sleep(3 * time.Millisecond)
}

func sendUnicodeString(text string) {
	for _, r := range text {
		sendUnicodeRune(r)
	}
}

func switchLayoutLinux() {
	detectDesktopTools()
	if qdbusBinary != "" {
		if err := exec.Command(qdbusBinary, "org.kde.keyboard", "/Layouts", "switchToNextLayout").Run(); err == nil {
			return
		}
	}
	emitKey(KEY_LEFTMETA, 1)
	time.Sleep(5 * time.Millisecond)
	tapKey(KEY_SPACE)
	emitKey(KEY_LEFTMETA, 0)
}

func isRussianLayoutLinux() bool {
	detectDesktopTools()
	if qdbusBinary != "" {
		if out, err := exec.Command(qdbusBinary, "org.kde.keyboard", "/Layouts", "getLayout").Output(); err == nil {
			return strings.Contains(strings.ToLower(strings.TrimSpace(string(out))), "ru")
		}
	}
	return lastFinalLang == "ru"
}

func getClipboardLinux() (string, bool) {
	if out, err := exec.Command("wl-paste", "--no-newline").Output(); err == nil {
		return string(out), true
	}
	if out, err := exec.Command("xclip", "-o", "-selection", "clipboard").Output(); err == nil {
		return string(out), true
	}
	return "", false
}

func setClipboardLinux(text string) {
	if _, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		cmd.Run()
		return
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-i", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		cmd.Run()
	}
}

func sendCtrlKeyLinux(key uint16) {
	emitKey(KEY_LEFTSHIFT, 0)
	emitKey(KEY_RIGHTSHIFT, 0)
	emitKey(KEY_LEFTALT, 0)
	emitKey(KEY_RIGHTALT, 0)
	time.Sleep(10 * time.Millisecond)
	emitKey(KEY_LEFTCTRL, 1)
	time.Sleep(5 * time.Millisecond)
	tapKey(key)
	emitKey(KEY_LEFTCTRL, 0)
}

func getCharEn(code uint16, shift bool) string {
	char, ok := evToEn[code]
	if !ok {
		return ""
	}
	if shift {
		if val, has := evToEnShift[code]; has {
			return val
		}
		if code >= KEY_Q && code <= KEY_M && char >= "a" && char <= "z" {
			return strings.ToUpper(char)
		}
	}
	return char
}

func parseHotkeyLinux(hotkeyStr string) (code uint16, needShift bool, needAlt bool) {
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
		code = KEY_PAUSE
	case "f12":
		code = KEY_F12
	case "f9":
		code = KEY_F9
	case "f8":
		code = KEY_F8
	case "scrolllock":
		code = KEY_SCROLLLOCK
	case "capslock":
		code = KEY_CAPSLOCK
	default:
		code = KEY_PAUSE
	}
	return
}

func isModifierLinux(code uint16) bool {
	switch code {
	case KEY_LEFTSHIFT, KEY_RIGHTSHIFT, KEY_LEFTCTRL, KEY_RIGHTCTRL, KEY_LEFTALT, KEY_RIGHTALT, KEY_CAPSLOCK, KEY_LEFTMETA, KEY_RIGHTMETA:
		return true
	}
	return false
}

// --- ЯДРО ХУКА (ПЕРЕДАЧА В ENGINE) ---

func handleLinuxKeyEvent(ev linuxInputEvent) {
	// ⚡ Пропускаем все события в тестовом режиме
	if ev.Type == EV_KEY && IsTestMode() {
		return
	}
	if ev.Type != EV_KEY || atomic.LoadUint32(&isReplacing) == 1 {
		return
	}
	code := ev.Code

	if ev.Value == 0 {
		switch code {
		case KEY_LEFTSHIFT:
			lShiftDown = false
			shiftReleased = true
		case KEY_RIGHTSHIFT:
			rShiftDown = false
			shiftReleased = true
		case KEY_LEFTCTRL:
			lCtrlDown = false
		case KEY_RIGHTCTRL:
			rCtrlDown = false
		case KEY_LEFTALT:
			lAltDown = false
		case KEY_RIGHTALT:
			rAltDown = false
		case KEY_LEFTMETA:
			lMetaDown = false
		case KEY_RIGHTMETA:
			rMetaDown = false
		}
		return
	}
	if ev.Value != 1 {
		return
	}

	switch code {
	case KEY_LEFTSHIFT:
		lShiftDown = true
	case KEY_RIGHTSHIFT:
		rShiftDown = true
	case KEY_LEFTCTRL:
		lCtrlDown = true
	case KEY_RIGHTCTRL:
		rCtrlDown = true
	case KEY_LEFTALT:
		lAltDown = true
	case KEY_RIGHTALT:
		rAltDown = true
	case KEY_LEFTMETA:
		lMetaDown = true
	case KEY_RIGHTMETA:
		rMetaDown = true
	case KEY_CAPSLOCK:
		capsLockState = !capsLockState
	}

	activeProc := getActiveProcessLinux()
	if activeProc != lastActiveProc {
		if lastActiveProc != "" {
			EngineClearContext()
		}
		lastActiveProc = activeProc
	}
	if activeProc != "" {
		settingsMutex.RLock()
		blacklist := strings.ToLower(settings.Blacklist)
		settingsMutex.RUnlock()
		for _, b := range strings.Split(blacklist, "\n") {
			b = strings.TrimSuffix(strings.TrimSpace(b), ".exe")
			if b != "" && (activeProc == b || strings.Contains(activeProc, b)) {
				return
			}
		}
	}

	if code == KEY_LEFTSHIFT || code == KEY_RIGHTSHIFT {
		if shiftReleased {
			EngineHandleDoubleShift()
			shiftReleased = false
		}
	} else if code != KEY_BACKSPACE {
		EngineResetCounters()
	}

	settingsMutex.RLock()
	targetCode, targetNeedShift, targetNeedAlt := parseHotkeyLinux(settings.Hotkey)
	caseCode, caseNeedShift, caseNeedAlt := parseHotkeyLinux(settings.CaseHotkey)
	settingsMutex.RUnlock()

	isShift := lShiftDown || rShiftDown
	isAlt := lAltDown || rAltDown
	isCtrl := lCtrlDown || rCtrlDown
	isMeta := lMetaDown || rMetaDown

	// ⚡ Сброс слова при смене раскладки через модификаторы (Ctrl/Alt/Win)
	if (isCtrl || isAlt || isMeta) && code != targetCode && code != caseCode {
		EngineClearContext()
		if isCtrl {
			return
		}
	}

	if code == caseCode && isShift == caseNeedShift && isAlt == caseNeedAlt {
		go EngineHandleCaseToggle()
		return
	}
	if code == targetCode && isShift == targetNeedShift && isAlt == targetNeedAlt {
		go EngineHandleManualSwitch()
		return
	}

	charEn := getCharEn(code, isShift)

	if code == KEY_SPACE {
		EngineHandleSpace()
	} else if code == KEY_ENTER {
		EngineHandleEnter()
	} else if code == KEY_TAB {
		EngineHandleTab()
	} else if code == KEY_BACKSPACE {
		EngineHandleBackspace()
	} else {
		charRu := evToRu[code]
		if isShift {
			if val, ok := evToRuShift[code]; ok {
				charRu = val
			} else if code >= KEY_Q && code <= KEY_M {
				charRu = strings.ToUpper(charRu)
			}
		}

		if charEn != "" {
			EngineAddChar(charEn, charRu, isShift, capsLockState)
		} else if !isModifierLinux(code) {
			EngineClearContext()
		}
	}
}

func findKeyboards() []string {
	var keyboards []string
	matches, _ := filepath.Glob("/dev/input/event*")
	for _, path := range matches {
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		var nameBuf [256]byte
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), EVIOCGNAME_256, uintptr(unsafe.Pointer(&nameBuf[0])))
		name := string(bytes.TrimRight(nameBuf[:], "\x00"))
		if strings.Contains(name, "GoSwitcher") {
			syscall.Close(fd)
			continue
		}

		var evBits [1]uint64
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), EVIOCGBIT_EV, uintptr(unsafe.Pointer(&evBits[0])))
		if (evBits[0] & (1 << EV_KEY)) != 0 {
			var keyBits [12]uint64
			syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), EVIOCGBIT_KEY, uintptr(unsafe.Pointer(&keyBits[0])))
			if (keyBits[KEY_A/64]&(1<<(KEY_A%64))) != 0 && (keyBits[KEY_SPACE/64]&(1<<(KEY_SPACE%64))) != 0 {
				var ledBits [1]uint64
				syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), EVIOCGLED, uintptr(unsafe.Pointer(&ledBits[0])))
				if (ledBits[0] & (1 << LED_CAPSL)) != 0 {
					capsLockState = true
				}
				keyboards = append(keyboards, path)
			}
		}
		syscall.Close(fd)
	}
	return keyboards
}

func startHook() {
	if hookQuit == nil {
		hookQuit = make(chan struct{})
	}
	if err := initUinput(); err != nil {
		fmt.Printf("[Linux Hook] Ошибка uinput: %v\n", err)
		return
	}
	keyboards := findKeyboards()
	for _, devPath := range keyboards {
		atomic.AddInt32(&hookKeys, 1)
		go func(path string) {
			defer atomic.AddInt32(&hookKeys, -1)
			fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
			if err != nil {
				return
			}
			defer syscall.Close(fd)
			evSize := int(unsafe.Sizeof(linuxInputEvent{}))
			buf := make([]byte, evSize*16)
			for {
				n, err := syscall.Read(fd, buf)
				if err != nil || n < evSize {
					select {
					case <-hookQuit:
						return
					case <-time.After(100 * time.Millisecond):
						continue
					}
				}
				for offset := 0; offset+evSize <= n; offset += evSize {
					select {
					case <-hookQuit:
						return
					default:
					}
					var ev linuxInputEvent
					binary.Read(bytes.NewReader(buf[offset:offset+evSize]), binary.LittleEndian, &ev)
					handleLinuxKeyEvent(ev)
				}
			}
		}(devPath)
	}
}

// stopHook останавливает все горутины хука и закрает дескрипторы
func stopHook() {
	if hookQuit != nil {
		close(hookQuit)
	}
	// Ждём завершения всех горутин хука
	for atomic.LoadInt32(&hookKeys) > 0 {
		time.Sleep(10 * time.Millisecond)
	}
	// Закрываем uinputFd
	if uinputFd >= 0 {
		syscall.Close(uinputFd)
		uinputFd = -1
	}
}
