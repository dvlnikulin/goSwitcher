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
	"unicode"
	"unsafe"
)

// Стандартные константы Linux Input (linux/input-event-codes.h)
const (
	EV_SYN = 0x00
	EV_KEY = 0x01
	EV_LED = 0x11

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

	// ioctl константы для evdev / uinput
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
		Bustype uint16
		Vendor  uint16
		Product uint16
		Version uint16
	}
	Name         [80]byte
	FfEffectsMax uint32
}

type KeyEvent struct {
	Code  uint16
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
	uinputFd                int = -1
	currentWord             []KeyEvent
	isReplacing             uint32
	lastWord                []KeyEvent
	typedSpaceAfterLastWord bool
	lastRenderedWord        string
	lastRenderedSuffix      string
	lastAutoCorrection      *CorrectionData
	backspaceCount          int
	lastShiftTime           time.Time
	shiftReleased           bool   = true
	lastFinalLang           string = "en"

	// Кэш активного процесса (Wayland / X11)
	cachedActiveProc     string
	lastActiveProc       string
	lastProcCheckTime    time.Time
	hasKdotool           bool
	hasXdotool           bool
	qdbusBinary          string
	waylandToolsDetected bool

	// Состояние модификаторов в реальном времени
	lShiftDown, rShiftDown bool
	lCtrlDown, rCtrlDown   bool
	lAltDown, rAltDown     bool
	capsLockState          bool
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

var hexToKey = map[rune]uint16{
	'0': KEY_0, '1': KEY_1, '2': KEY_2, '3': KEY_3, '4': KEY_4,
	'5': KEY_5, '6': KEY_6, '7': KEY_7, '8': KEY_8, '9': KEY_9,
	'a': KEY_A, 'b': KEY_B, 'c': KEY_C, 'd': KEY_D, 'e': KEY_E, 'f': KEY_F,
}

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

// Нативное определение активного окна/процесса для KDE Plasma Wayland (KWin) и X11
func getActiveProcessLinux() string {
	if time.Since(lastProcCheckTime) < 250*time.Millisecond {
		return cachedActiveProc
	}
	lastProcCheckTime = time.Now()
	detectDesktopTools()

	var detected string

	// 1. KDE Plasma Wayland: kdotool (быстрый нативный инструмент для KWin)
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

	// 2. KDE Plasma Wayland: прямой запрос через KWin Scripting D-Bus (если kdotool не установлен)
	if detected == "" && qdbusBinary != "" && os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		detected = queryKWinActiveWindowViaDBus()
	}

	// 3. X11 / XWayland фоллбэк через xdotool
	if detected == "" && hasXdotool {
		if out, err := exec.Command("xdotool", "getactivewindow", "getwindowpid").Output(); err == nil {
			detected = readProcessNameByPID(string(out))
		}
	}

	cachedActiveProc = detected
	return detected
}

// Запуск одноразового KWin JS-скрипта через D-Bus для получения PID / resourceClass активного Wayland-окна
func queryKWinActiveWindowViaDBus() string {
	tmpFile, err := os.CreateTemp("", "goswitcher_kwin_*.js")
	if err != nil {
		return ""
	}
	scriptPath := tmpFile.Name()
	defer os.Remove(scriptPath)

	// Поддержка Plasma 6 (workspace.activeWindow) и Plasma 5 (workspace.activeClient)
	jsCode := `
var w = workspace.activeWindow || workspace.activeClient;
if (w) {
    print("GOSWITCHER_WIN:" + w.pid + ":" + w.resourceClass);
}
`
	tmpFile.WriteString(jsCode)
	tmpFile.Close()

	pluginName := "goswitcher_probe"
	out, err := exec.Command(qdbusBinary, "org.kde.KWin", "/Scripting", "org.kde.kwin.Scripting.loadScript", scriptPath, pluginName).Output()
	if err != nil {
		return ""
	}
	scriptId := strings.TrimSpace(string(out))
	if scriptId == "" {
		return ""
	}

	// В Plasma 6 путь /Scripting/Script<id>, в Plasma 5 /<id>
	scriptObj := "/Scripting/Script" + scriptId
	if err := exec.Command(qdbusBinary, "org.kde.KWin", scriptObj, "org.kde.kwin.Script.run").Run(); err != nil {
		scriptObj = "/" + scriptId
		exec.Command(qdbusBinary, "org.kde.KWin", scriptObj, "org.kde.kwin.Script.run").Run()
	}
	exec.Command(qdbusBinary, "org.kde.KWin", "/Scripting", "org.kde.kwin.Scripting.unloadScript", pluginName).Run()

	// Читаем свежую запись из journalctl за последнюю секунду
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

// Инициализация виртуальной клавиатуры uinput
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

func simulateBackspace() {
	tapKey(KEY_BACKSPACE)
}

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
			s := strings.ToLower(strings.TrimSpace(string(out)))
			return strings.Contains(s, "ru")
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

func tryTransformSelection(mode string) bool {
	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(30 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	backup, hadBackup := getClipboardLinux()
	setClipboardLinux("")

	sendCtrlKeyLinux(KEY_C)
	time.Sleep(75 * time.Millisecond)

	selected, ok := getClipboardLinux()
	if !ok || selected == "" {
		if hadBackup {
			setClipboardLinux(backup)
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
			setClipboardLinux(backup)
		}
		return true
	}

	setClipboardLinux(transformed)
	time.Sleep(20 * time.Millisecond)
	sendCtrlKeyLinux(KEY_V)
	time.Sleep(75 * time.Millisecond)

	if mode == "layout" {
		isRu := isRussianLayoutLinux()
		if (isRu && switchedToEn) || (!isRu && !switchedToEn) {
			switchLayoutLinux()
		}
	}

	if hadBackup {
		setClipboardLinux(backup)
	}
	currentWord = currentWord[:0]
	lastWord = lastWord[:0]
	lastRenderedWord = ""
	return true
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
	isAllLower, isAllUpper, hasLetters := true, true, false
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
		charEn := evToEn[ev.Code]
		charRu := evToRu[ev.Code]
		if ev.Caps {
			hasCapsLock = true
		}
		if ev.Shift {
			if val, ok := evToEnShift[ev.Code]; ok {
				charEn = val
			}
			if val, ok := evToRuShift[ev.Code]; ok {
				charRu = val
			}
		}
		if ev.Code >= KEY_1 && ev.Code <= KEY_0 {
			hasDigit = true
		}
		if charEn >= "a" && charEn <= "z" {
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
			switch ev.Code {
			case KEY_SEMICOLON, KEY_COMMA, KEY_DOT, KEY_GRAVE, KEY_LEFTBRACE, KEY_RIGHTBRACE, KEY_APOSTROPHE:
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

func replaceText(deletions int, replacement string, appendChar string, switchLang bool) {
	lastRenderedWord = replacement
	lastRenderedSuffix = appendChar

	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(25 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	time.Sleep(15 * time.Millisecond)
	totalDel := deletions
	if appendChar != "" {
		totalDel += len([]rune(appendChar))
	}
	for i := 0; i < totalDel; i++ {
		simulateBackspace()
		time.Sleep(3 * time.Millisecond)
	}

	time.Sleep(10 * time.Millisecond)
	sendUnicodeString(replacement + appendChar)
	time.Sleep(25 * time.Millisecond)

	if switchLang {
		switchLayoutLinux()
	}
}

func handleCaseToggle() {
	if tryTransformSelection("case") {
		return
	}
	isRu := isRussianLayoutLinux()
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

	isRu := isRussianLayoutLinux()
	deletions := len(currentWord)

	if isITPattern(wordEn) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
		}
		return
	}

	if hasDigit || (hasUpper && hasLower && !hasCapsLock) {
		return
	}

	firstCode := currentWord[0].Code
	isFlag := firstCode == KEY_MINUS
	isPath := firstCode == KEY_BACKSLASH || firstCode == KEY_SLASH

	if (isFlag && blockFlags) || (isPath && blockPaths) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
		}
		return
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

	isRu := isRussianLayoutLinux()
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
				OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
				Corrected: wordRu, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
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
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
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
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			backspaceCount = 0
			go replaceText(deletions, wordEn, appendChar, true)
		} else {
			lastFinalLang = "en"
		}
		return
	}

	if !isManual {
		if hasDigit || (hasUpper && hasLower && !hasCapsLock) {
			return
		}
	}
	if len(cEn) < 2 {
		return
	}

	if len(wordData) > 0 {
		firstCode := wordData[0].Code
		isFlag := firstCode == KEY_MINUS
		isPath := firstCode == KEY_BACKSLASH || firstCode == KEY_SLASH
		if (isFlag && blockFlags) || (isPath && blockPaths) {
			if isRu {
				lastFinalLang = "en"
				lastAutoCorrection = &CorrectionData{
					OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
					Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
				}
				incrementLayoutCorrection(len([]rune(cRu)))
				backspaceCount = 0
				go replaceText(deletions, wordEn, appendChar, true)
			} else {
				lastFinalLang = "en"
			}
			return
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
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: correctedEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
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
					OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
					Corrected: correctedRu, AppendChar: appendChar, SwitchedLang: false, Timestamp: time.Now(),
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
				OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
				Corrected: correctedRu, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
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
					OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
					Corrected: correctedEn, AppendChar: appendChar, SwitchedLang: false, Timestamp: time.Now(),
				}
				incrementTypoCorrection(len([]rune(cEn)))
				backspaceCount = 0
				go replaceText(deletions, correctedEn, appendChar, false)
			}
		}
	}
}

func isModifierLinux(code uint16) bool {
	switch code {
	case KEY_LEFTSHIFT, KEY_RIGHTSHIFT, KEY_LEFTCTRL, KEY_RIGHTCTRL, KEY_LEFTALT, KEY_RIGHTALT, KEY_CAPSLOCK, KEY_LEFTMETA:
		return true
	}
	return false
}

func handleLinuxKeyEvent(ev linuxInputEvent) {
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
	case KEY_CAPSLOCK:
		capsLockState = !capsLockState
	}

	// Проверка смены окна и Blacklist (Wayland KWin + X11)
	activeProc := getActiveProcessLinux()
	if activeProc != lastActiveProc {
		if lastActiveProc != "" {
			currentWord = currentWord[:0]
			lastWord = lastWord[:0]
			lastRenderedWord = ""
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

	// Double Shift для отмены автозамены
	if code == KEY_LEFTSHIFT || code == KEY_RIGHTSHIFT {
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
		return
	} else if code != KEY_BACKSPACE {
		lastShiftTime = time.Time{}
		backspaceCount = 0
	}

	if code == KEY_BACKSPACE {
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
	targetCode, targetNeedShift, targetNeedAlt := parseHotkeyLinux(settings.Hotkey)
	caseCode, caseNeedShift, caseNeedAlt := parseHotkeyLinux(settings.CaseHotkey)
	liveTypoEnabled := settings.LiveTypoEnabled
	liveTypoChar := settings.LiveTypoChar
	settingsMutex.RUnlock()

	if liveTypoChar < 3 {
		liveTypoChar = 4
	}

	isShift := lShiftDown || rShiftDown
	isAlt := lAltDown || rAltDown
	isCtrl := lCtrlDown || rCtrlDown

	if isCtrl && code != targetCode && code != caseCode {
		return
	}

	if code == caseCode && isShift == caseNeedShift && isAlt == caseNeedAlt {
		go handleCaseToggle()
		return
	}

	if code == targetCode && isShift == targetNeedShift && isAlt == targetNeedAlt {
		go handleManualLayoutSwitch()
		return
	}

	charEn := getCharEn(code, isShift)

	if code == KEY_SPACE {
		processWord(" ", false)
	} else if code == KEY_ENTER {
		processWord("\n", false)
	} else if code == KEY_TAB {
		processWord("\t", false)
	} else if code == KEY_BACKSPACE {
		if len(currentWord) > 0 {
			currentWord = currentWord[:len(currentWord)-1]
		} else {
			lastWord = lastWord[:0]
			lastRenderedWord = ""
		}
	} else if charEn != "" && isBoundaryEn(charEn) {
		processWord(charEn, false)
	} else if charEn != "" {
		currentWord = append(currentWord, KeyEvent{Code: code, Shift: isShift, Caps: capsLockState})
		if liveTypoEnabled && len(currentWord) >= liveTypoChar {
			processLiveTypo()
		}
	} else if !isModifierLinux(code) {
		currentWord = currentWord[:0]
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
			hasA := (keyBits[KEY_A/64] & (1 << (KEY_A % 64))) != 0
			hasSpace := (keyBits[KEY_SPACE/64] & (1 << (KEY_SPACE % 64))) != 0
			if hasA && hasSpace {
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
	if err := initUinput(); err != nil {
		fmt.Printf("[Linux Hook] Ошибка инициализации /dev/uinput: %v\n", err)
		return
	}

	keyboards := findKeyboards()
	for _, devPath := range keyboards {
		go func(path string) {
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
					time.Sleep(100 * time.Millisecond)
					continue
				}
				for offset := 0; offset+evSize <= n; offset += evSize {
					var ev linuxInputEvent
					binary.Read(bytes.NewReader(buf[offset:offset+evSize]), binary.LittleEndian, &ev)
					handleLinuxKeyEvent(ev)
				}
			}
		}(devPath)
	}
}
