//go:build linux && e2e

package e2e

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ─────────────────────────────────────────────────────────────
// Linux E2E — /dev/uinput (evdev) + D-Bus / X11
// ─────────────────────────────────────────────────────────────

// Linux Event Types
const (
	EV_KEY    = 0x01
	EV_REL    = 0x02
	EV_ABS    = 0x03
	EV_MSC    = 0x04
	EV_SYN    = 0x00

	SYN_REPORT   = 0x00
	SYNC_FRAME   = 11

	// Key codes (linux/input-event-codes.h)
	KEY_BACKSPACE   = 14
	KEY_ENTER       = 28
	KEY_TAB         = 15
	KEY_SPACE       = 57
	KEY_LEFTSHIFT   = 42
	KEY_RIGHTSHIFT  = 54
	KEY_LEFTCTRL    = 29
	KEY_RIGHTCTRL   = 97
	KEY_LEFTALT     = 56
	KEY_RIGHTALT    = 100
	KEY_LEFTMETA    = 125
	KEY_RIGHTMETA   = 126
	KEY_CAPSLOCK    = 58
	KEY_SCROLLLOCK  = 70
	KEY_NUMLOCK     = 69
	KEY_ESCAPE      = 1
	KEY_DELETE      = 111
	KEY_HOME        = 102
	KEY_END         = 107
	KEY_LEFT        = 105
	KEY_RIGHT       = 106
	KEY_UP          = 103
	KEY_DOWN        = 108
	KEY_PAUSE       = 119
	KEY_PRINT       = 114

	// Linux input event size
	inputEventSize = int(unsafe.Sizeof(inputEvent{}))
)

// inputEvent — структура события evdev
type inputEvent struct {
	Ts  syscall.Timeval
	Type uint16
	Code uint16
	Value int32
}

// Uinput Constants
const (
	UI_SET_EVBIT   = 0x40045564
	UI_SET_KEYBIT  = 0x40045565
	UI_DEV_SETUP   = 0x405c5503
	UI_DEV_CREATE  = 0x5501
	UI_DEV_DESTROY = 0x5502

	BUS_USB   = 0x03
	BUS_VIRTUAL = 0x05
)

// uinputSetup — структура настройки uinput устройства
type uinputSetup struct {
	ID struct {
		Bustype, Vendor, Product, Version uint16
	}
	Name         [80]byte
	FfEffectsMax uint32
	AbsMax       [32]uint32 // ABS_MAX
}

// uinputFd — файловый дескриптор виртуального устройства
var uinputFd int = -1

// TestDeviceName — имя виртуального устройства для тестовых событий
const TestDeviceName = "GoSwitcher-E2E-Test"

// ─────────────────────────────────────────────────────────────
// Device Setup — uinput
// ─────────────────────────────────────────────────────────────

// OpenUinput — открывает /dev/uinput и создаёт виртуальную клавиатуру
func OpenUinput() error {
	if uinputFd >= 0 {
		return nil // уже открыт
	}

	// Открываем /dev/uinput
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("OpenUinput: cannot open /dev/uinput: %w", err)
	}
	uinputFd = fd

	// Устанавливаем биты событий
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_KEY)
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_SYN)

	// Устанавливаем биты клавиш
	for code := uint16(0); code < 256; code++ {
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_KEYBIT, uintptr(code))
	}

	// Настраиваем устройство
	var setup uinputSetup
	setup.ID.Bustype = BUS_VIRTUAL
	setup.ID.Vendor = 0x1234
	setup.ID.Product = 0x5678
	copy(setup.Name[:], TestDeviceName)

	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_SETUP, uintptr(unsafe.Pointer(&setup)))
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_CREATE, 0)

	time.Sleep(150 * time.Millisecond)

	fmt.Println("[E2E] uinput устройство создано:", TestDeviceName)
	return nil
}

// CloseUinput — закрывает uinput устройство
func CloseUinput() {
	if uinputFd < 0 {
		return
	}

	// Уничтожаем устройство
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(uinputFd), UI_DEV_DESTROY, 0)
	syscall.Close(uinputFd)
	uinputFd = -1
	fmt.Println("[E2E] uinput устройство удалено")
}

// ─────────────────────────────────────────────────────────────
// Event Emission
// ─────────────────────────────────────────────────────────────

// emitKey — отправить событие клавиши через uinput
func emitKey(code uint16, value int32) error {
	if uinputFd < 0 {
		return fmt.Errorf("emitKey: uinput не открыт")
	}

	ev := inputEvent{
		Type:  EV_KEY,
		Code:  code,
		Value: value,
	}
	syn := inputEvent{
		Type:  EV_SYN,
		Code:  SYN_REPORT,
		Value: 0,
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, ev)
	binary.Write(&buf, binary.LittleEndian, syn)

	_, err := syscall.Write(uinputFd, buf.Bytes())
	return err
}

// tapKey — нажатие+отпускание (KEY_DOWN + KEY_UP)
func tapKey(code uint16) error {
	if err := emitKey(code, 1); err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	if err := emitKey(code, 0); err != nil {
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// High-level Input Helpers (Linux)
// ─────────────────────────────────────────────────────────────

// SendLinuxTestKey — отправить тестовую клавишу через uinput
func SendLinuxTestKey(code uint16, delay time.Duration) {
	if err := emitKey(code, 1); err != nil {
		fmt.Printf("[E2E] ERROR emitKey DOWN %d: %v\n", code, err)
		return
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	if err := emitKey(code, 0); err != nil {
		fmt.Printf("[E2E] ERROR emitKey UP %d: %v\n", code, err)
		return
	}
	if delay > 0 {
		time.Sleep(delay)
	}
}

// SendLinuxTestString — отправить строку по символам (поиск rune в maps)
// Использует эмуляцию Ctrl+Shift+U + hex для Unicode
func SendLinuxTestString(text string, delay time.Duration) {
	for _, r := range text {
		if err := sendUnicodeRune(r); err != nil {
			fmt.Printf("[E2E] ERROR sendUnicodeRune %c: %v\n", r, err)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// sendUnicodeRune — эмулировать Ctrl+Shift+U + hex для Unicode (как в GNOME/KDE)
func sendUnicodeRune(r rune) error {
	// Send Ctrl+Shift+U
	if err := emitKey(KEY_LEFTCTRL, 1); err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	if err := emitKey(KEY_LEFTSHIFT, 1); err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	if err := tapKey(KEY_U); err != nil {
		return err
	}
	// Release Ctrl+Shift
	if err := emitKey(KEY_LEFTSHIFT, 0); err != nil {
		return err
	}
	if err := emitKey(KEY_LEFTCTRL, 0); err != nil {
		return err
	}
	time.Sleep(5 * time.Millisecond)

	// Send hex digits
	hexStr := fmt.Sprintf("%04x", r)
	for _, h := range hexStr {
		var code uint16
		switch h {
		case '0': code = KEY_0
		case '1': code = KEY_1
		case '2': code = KEY_2
		case '3': code = KEY_3
		case '4': code = KEY_4
		case '5': code = KEY_5
		case '6': code = KEY_6
		case '7': code = KEY_7
		case '8': code = KEY_8
		case '9': code = KEY_9
		case 'a': code = KEY_A
		case 'b': code = KEY_B
		case 'c': code = KEY_C
		case 'd': code = KEY_D
		case 'e': code = KEY_E
		case 'f': code = KEY_F
		default:
			return fmt.Errorf("unknown hex char %c", h)
		}
		if err := tapKey(code); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// SendLinuxKeyCombo — комбинация клавиш
func SendLinuxKeyCombo(codes []uint16, delay time.Duration) {
	var mods, main uint16
	if len(codes) == 0 {
		return
	}
	if len(codes) == 1 {
		main = codes[0]
	} else {
		mods = codes[:len(codes)-1]
		main = codes[len(codes)-1]
	}

	for _, c := range mods {
		emitKey(c, 1)
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	emitKey(main, 1)
	if delay > 0 {
		time.Sleep(delay)
	}
	emitKey(main, 0)
	if delay > 0 {
		time.Sleep(delay)
	}
	for i := len(mods) - 1; i >= 0; i-- {
		emitKey(mods[i], 0)
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// SendLinuxCtrlKey — Ctrl+Key
func SendLinuxCtrlKey(key uint16, delay time.Duration) {
	SendLinuxKeyCombo([]uint16{KEY_LEFTCTRL, key}, delay)
}

// SendLinuxLeft — стрелка влево
func SendLinuxLeft(count int, delay time.Duration) {
	for i := 0; i < count; i++ {
		SendLinuxTestKey(KEY_LEFT, delay)
	}
}

// SendLinuxBackspace — Backspace (для double-shift undo)
func SendLinuxBackspace(delay time.Duration) {
	SendLinuxKeyCombo([]uint16{KEY_RIGHTSHIFT, KEY_BACKSPACE}, delay)
}

// ─────────────────────────────────────────────────────────────
// Layout Detection (Linux)
// ─────────────────────────────────────────────────────────────

// GetLayoutID — получить текущую раскладку через D-Bus (KDE) или X11
func GetLayoutID() string {
	sessionType := os.Getenv("XDG_SESSION_TYPE")

	// Wayland / KDE — D-Bus
	if sessionType == "wayland" || sessionType == "" {
		if out, err := exec.Command("qdbus", "org.kde.keyboard", "/Layouts", "getLayout").Output(); err == nil {
			layout := strings.TrimSpace(string(out))
			if strings.Contains(strings.ToLower(layout), "ru") {
				return "RU"
			}
			return "EN"
		}
	}

	// X11 — xkb
	if sessionType == "x11" || sessionType == "" {
		if _, err := exec.LookPath("setxkbmap"); err == nil {
			if out, err := exec.Command("setxkbmap", "-query").Output(); err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(line, "layout:") {
						layout := strings.TrimSpace(strings.TrimPrefix(line, "layout:"))
						if strings.Contains(strings.ToLower(layout), "ru") {
							return "RU"
						}
						return "EN"
					}
				}
			}
		}
		if _, err := exec.LookPath("xkblayout-state"); err == nil {
			if out, err := exec.Command("xkblayout-state", "get").Output(); err == nil {
				if strings.Contains(strings.ToLower(string(out)), "ru") {
					return "RU"
				}
				return "EN"
			}
		}
	}

	return ""
}

// SwitchLayout — переключить раскладку (Alt+Shift)
func SwitchLayout() {
	SendLinuxKeyCombo([]uint16{KEY_LEFTALT, KEY_LEFTSHIFT}, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
}

// SetLayoutTo — попытаться установить конкретную раскладку
func SetLayoutTo(layout string) {
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	if sessionType == "wayland" {
		// KDE — D-Bus
		if _, err := exec.LookPath("qdbus"); err == nil {
			var target string
			if strings.Contains(strings.ToLower(layout), "ru") {
				target = "ru"
			} else {
				target = "us"
			}
			exec.Command("qdbus", "org.kde.keyboard", "/Layouts", "setLayout", target).Run()
			time.Sleep(200 * time.Millisecond)
			return
		}
	}

	// X11 — setxkbmap
	if _, err := exec.LookPath("setxkbmap"); err == nil {
		var target string
		if strings.Contains(strings.ToLower(layout), "ru") {
			target = "ru"
		} else {
			target = "us"
		}
		exec.Command("setxkbmap", "-layout", target).Run()
		time.Sleep(200 * time.Millisecond)
		return
	}

	// Fallback — Alt+Shift
	if strings.Contains(strings.ToLower(layout), "ru") != (GetLayoutID() == "RU") {
		SwitchLayout()
	}
}

// ─────────────────────────────────────────────────────────────
// Window Management (Linux — X11/Wayland)
// ─────────────────────────────────────────────────────────────

// FocusWindow — попытаться сфокусировать окно (заглушка, т.к. Wayland не позволяет)
func FocusWindow(name string) bool {
	// X11 — xdotool
	if _, err := exec.LookPath("xdotool"); err == nil {
		if out, err := exec.Command("xdotool", "search", "--name", name).Output(); err == nil {
			pid := strings.TrimSpace(string(out))
			if pid != "" {
				exec.Command("xdotool", "windowactivate", pid).Run()
				time.Sleep(300 * time.Millisecond)
				return true
			}
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────
// Focus Lost — эмуляция Alt+Tab
// ─────────────────────────────────────────────────────────────

// AltTab — Alt+Tab (потеря фокуса)
func AltTab() {
	SendLinuxKeyCombo([]uint16{KEY_LEFTALT, KEY_TAB}, 10*time.Millisecond)
}

// AltTabRelease — Alt+Tab (удержание Alt)
func AltTabRelease() {
	if err := emitKey(KEY_LEFTALT, 0); err != nil {
		fmt.Printf("[E2E] ERROR release Alt: %v\n", err)
	}
	time.Sleep(200 * time.Millisecond)
}

// ─────────────────────────────────────────────────────────────
// Timing (Linux)
// ─────────────────────────────────────────────────────────────

// MinTypingDelay — минимальная задержка между нажатиями (15ms для Linux)
const MinTypingDelay = 15 * time.Millisecond

// KeyUpDelay — задержка между KeyDown и KeyUp (3ms)
const KeyUpDelay = 3 * time.Millisecond

// PasteDelay — задержка после вставки в буфер обмена (150ms)
const PasteDelay = 150 * time.Millisecond

// LayoutSwitchDelay — задержка после смены раскладки (300ms)
const LayoutSwitchDelay = 300 * time.Millisecond

// AutoSwitchDelay — задержка после автопереключения (500ms для Linux)
const AutoSwitchDelay = 500 * time.Millisecond

// BackspaceGap — задержка после серии Backspace перед вставкой
const BackspaceGap = 50 * time.Millisecond

// ─────────────────────────────────────────────────────────────
// Prerequisites — проверка прав и устройств
// ─────────────────────────────────────────────────────────────

// CheckPrerequisites — проверка готовности среды
func CheckPrerequisites() error {
	// Проверяем /dev/uinput
	if _, err := os.Stat("/dev/uinput"); err != nil {
		return fmt.Errorf("/dev/uinput не существует. Запустите с sudo или добавьте в группу input: usermod -aG input $USER")
	}

	// Проверяем, что устройство доступно
	if _, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0); err != nil {
		return fmt.Errorf("/dev/uinput недоступен. Проверьте права: chmod 666 /dev/uinput")
	}

	return nil
}

// SetupTestDevice — подготовить тестовое устройство
func SetupTestDevice() error {
	if err := OpenUinput(); err != nil {
		return err
	}

	// Проверяем, что устройство появилось
	time.Sleep(200 * time.Millisecond)

	// Находим путь к новому устройству
	matches, _ := filepath.Glob("/dev/input/event*")
	for _, path := range matches {
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		var nameBuf [256]byte
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x81004506, uintptr(unsafe.Pointer(&nameBuf[0])))
		name := strings.TrimSpace(string(bytes.TrimRight(nameBuf[:], "\x00")))
		syscall.Close(fd)

		if name == TestDeviceName {
			fmt.Printf("[E2E] Найдено тестовый устройство: %s (%s)\n", name, path)
			return nil
		}
	}

	fmt.Printf("[E2E] WARNING: тестовое устройство %s не найдено в /dev/input/\n", TestDeviceName)
	fmt.Println("[E2E] Свитчер будет фильтровать по dwExtraInfo (через SWITCHER_TEST_MODE)")
	return nil
}

// CleanupTestDevice — удалить тестовое устройство
func CleanupTestDevice() {
	CloseUinput()
}
