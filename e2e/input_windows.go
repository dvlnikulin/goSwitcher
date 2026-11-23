//go:build windows && e2e

package e2e

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

// ─────────────────────────────────────────────────────────────
// Windows E2E — SendInput (Win32 API) + Layout Management
// ─────────────────────────────────────────────────────────────

// dwExtraInfo — сигнатура тестовых событий, чтобы свитчер мог отличить их от своих.
// Значение 0x53574954 = "SWIT" в ASCII.
const TEST_EVENT_SIGNATURE = uint32(0x53574954)

// ─────────────────────────────────────────────────────────────
// Event Factories — с тестовой сигнатурой
// ─────────────────────────────────────────────────────────────

// testKeyDown — KeyDown с TEST_EVENT_SIGNATURE в dwExtraInfo
func testKeyDown(vk uint16) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        vk,
			DwExtraInfo: uintptr(TEST_EVENT_SIGNATURE),
		},
	}
}

// testKeyUp — KeyUp с TEST_EVENT_SIGNATURE
func testKeyUp(vk uint16) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        vk,
			Flags:     KEYEVENTF_KEYUP,
			DwExtraInfo: uintptr(TEST_EVENT_SIGNATURE),
		},
	}
}

// testCharDown — Unicode-символ KeyDown
func testCharDown(r rune) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        0,
			Scan:      0,
			Flags:     KEYEVENTF_UNICODE,
			DwExtraInfo: uintptr(TEST_EVENT_SIGNATURE),
		},
	}
}

// testCharUp — Unicode-символ KeyUp
func testCharUp(r rune) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        0,
			Scan:      0,
			Flags:     KEYEVENTF_UNICODE | KEYEVENTF_KEYUP,
			DwExtraInfo: uintptr(TEST_EVENT_SIGNATURE),
		},
	}
}

// normalKeyDown — KeyDown без сигнатуры (для свитчера — как от пользователя)
func normalKeyDown(vk uint16) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{Vk: vk},
	}
}

// normalKeyUp — KeyUp без сигнатуры
func normalKeyUp(vk uint16) INPUT {
	return INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{Vk: vk, Flags: KEYEVENTF_KEYUP},
	}
}

// ─────────────────────────────────────────────────────────────
// High-level Input Helpers
// ─────────────────────────────────────────────────────────────

// SendTestKey — отправить одну тестовую клавишу (нажмите+отпустил)
func SendTestKey(vk uint16, delay time.Duration) {
	sendInputRaw([]INPUT{testKeyDown(vk)})
	if delay > 0 {
		time.Sleep(delay)
	}
	sendInputRaw([]INPUT{testKeyUp(vk)})
	if delay > 0 {
		time.Sleep(delay)
	}
}

// SendTestString — отправить строку как набор Unicode-символов
func SendTestString(text string, delay time.Duration) {
	for _, r := range text {
		sendInputRaw([]INPUT{testCharDown(r)})
		if delay > 0 {
			time.Sleep(delay)
		}
		sendInputRaw([]INPUT{testCharUp(r)})
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// SendKeyCombo — комбинация клавиш [VK_L, VK_R, ...]
// Порядок: сначала нажимаются все, потом отпускаются в обратном порядке.
// Если в списке есть non-modifier — он нажимается и отпускается отдельно.
func SendKeyCombo(keys []uint16, delay time.Duration) {
	// Разделяем модификаторы и основную клавишу
	var mods []uint16
	var main uint16
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		main = keys[0]
	} else {
		mods = keys[:len(keys)-1]
		main = keys[len(keys)-1]
	}

	for _, vk := range mods {
		sendInputRaw([]INPUT{testKeyDown(vk)})
		if delay > 0 {
			time.Sleep(delay)
		}
	}

	sendInputRaw([]INPUT{testKeyDown(main)})
	if delay > 0 {
		time.Sleep(delay)
	}
	sendInputRaw([]INPUT{testKeyUp(main)})
	if delay > 0 {
		time.Sleep(delay)
	}

	for i := len(mods) - 1; i >= 0; i-- {
		sendInputRaw([]INPUT{testKeyUp(mods[i])})
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// SendCtrlKey — быстрый Ctrl+Key
func SendCtrlKey(key rune, delay time.Duration) {
	SendKeyCombo([]uint16{VK_CONTROL, uint16(key)}, delay)
}

// SendShiftBackspace — Shift+Backspace (Double Shift для undo)
func SendShiftBackspace(delay time.Duration) {
	SendKeyCombo([]uint16{VK_SHIFT, VK_BACK}, delay)
}

// SendLeft — стрелка влево
func SendLeft(count int, delay time.Duration) {
	for i := 0; i < count; i++ {
		SendTestKey(VK_LEFT, delay)
	}
}

// ─────────────────────────────────────────────────────────────
// Layout Management
// ─────────────────────────────────────────────────────────────

// GetForegroundLayoutID — получить ID раскладки активного окна
func GetForegroundLayoutID() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	threadID, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	layout, _, _ := procGetKeyboardLayout.Call(threadID)
	return fmt.Sprintf("%08X", layout&0xFFFF0000)
}

// SetForegroundLayout — установить раскладку в активном окне
func SetForegroundLayout(klid string) bool {
	klidW, _ := syscall.UTF16FromString(klid)
	ret, _, _ := procActivateKeyboardLayout.Call(
		uintptr(unsafe.Pointer(&klidW[0])),
		0, // KLF_ACTIVATE
	)
	return ret != 0
}

// ─────────────────────────────────────────────────────────────
// IME — отключение Imm32 для чистоты тестов
// ─────────────────────────────────────────────────────────────

// DisableIME — отключить IME (нужно для корректной работы SendInput с русским текстом)
func DisableIME() {
	procImmDisableIME.Call()
}

// ─────────────────────────────────────────────────────────────
// Timing — задержки для синхронизации с GUI
// ─────────────────────────────────────────────────────────────

// MinTypingDelay — минимальная задержка между нажатиями (10ms)
const MinTypingDelay = 10 * time.Millisecond

// KeyUpDelay — задержка между KeyDown и KeyUp (5ms)
const KeyUpDelay = 5 * time.Millisecond

// PasteDelay — задержка после вставки в буфер обмена (100ms)
const PasteDelay = 100 * time.Millisecond

// LayoutSwitchDelay — задержка после смены раскладки (200ms)
const LayoutSwitchDelay = 200 * time.Millisecond

// AutoSwitchDelay — задержка после автопереключения (300ms)
const AutoSwitchDelay = 300 * time.Millisecond
