//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Test: Mixed Input Mode (English + Russian)
// ─────────────────────────────────────────────────────────────

func TestMixedInputMode(t *testing.T) {
	printStep("Test: mixed input (English words + Russian in EN layout)")

	if err := SetTestMode(true); err != nil {
		t.Fatalf("SetTestMode: %v", err)
	}
	t.Cleanup(func() {
		SetTestMode(false)
		time.Sleep(100 * time.Millisecond)
	})

	_, editHwnd := launchTargetWindows()
	if editHwnd == 0 {
		t.Skip("Notepad not available (headless environment)")
	}

	// Включаем свитчер
	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Смешанный ввод: "hello world привет как дела"
	// "hello world" — английские слова (должны остаться)
	// "привет" — "ghbdtn" в английской раскладке (должно переключиться)
	// "как" — "rfr" в английской раскладке (должно переключиться)
	// "дела" — "ltkf" в английской раскладке (должно переключиться)
	phrase := "hello world ghbdtn rfr ltkf"
	printStep(fmt.Sprintf("Phrase: '%s'", phrase))

	// Печатаем с задержкой 30ms на клавишу
	SendTestString(phrase, 30*time.Millisecond)
	time.Sleep(1 * time.Second)

	// Проверяем результат
	text := strings.TrimSpace(getEditText(uint32(editHwnd)))
	printStep(fmt.Sprintf("Result: '%s'", text))

	// Ожидаем: "hello world привет как дела"
	expectedWords := []string{
		"hello",    // английский, не должен переключиться
		"world",    // английский, не должен переключиться
		"привет",   // должен переключиться из "ghbdtn"
		"как",      // должен переключиться из "rfr"
		"дела",     // должен переключиться из "ltkf"
	}

	// Проверяем, что все ожидаемые слова присутствуют
	textLower := strings.ToLower(text)
	allFound := true
	for _, expected := range expectedWords {
		expectedLower := strings.ToLower(expected)
		if !strings.Contains(textLower, expectedLower) {
			allFound = false
			t.Errorf("  Not found: '%s'", expected)
		}
	}

	if !allFound {
		t.Errorf("Mixed input test failed:\n  Got:      '%s'\n  Expected to contain: %v", text, expectedWords)
	}

	// Проверяем, что английские слова остались без изменений
	if !strings.Contains(text, "hello") {
		t.Errorf("English word 'hello' was incorrectly converted: '%s'", text)
	}
	if !strings.Contains(text, "world") {
		t.Errorf("English word 'world' was incorrectly converted: '%s'", text)
	}

	// Проверяем, что русские слова переключились
	if !strings.Contains(text, "привет") {
		t.Errorf("Russian word 'привет' was not converted from 'ghbdtn': '%s'", text)
	}
	if !strings.Contains(text, "как") {
		t.Errorf("Russian word 'как' was not converted from 'rfr': '%s'", text)
	}
	if !strings.Contains(text, "дела") {
		t.Errorf("Russian word 'дела' was not converted from 'ltkf': '%s'", text)
	}

	printStep(fmt.Sprintf("✓ All checks passed"))
}

// TestMixedInputWithCode — mixed input with code, URLs, and Russian
func TestMixedInputWithCode(t *testing.T) {
	printStep("Test: mixed input with code and URLs")

	if err := SetTestMode(true); err != nil {
		t.Fatalf("SetTestMode: %v", err)
	}
	t.Cleanup(func() {
		SetTestMode(false)
	})

	_, editHwnd := launchTargetWindows()
	if editHwnd == 0 {
		t.Skip("Notepad not available (headless environment)")
	}

	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Смешанный ввод: код + URL + русский текст
	phrase := "hello world https://site.ru ghbdtn rfr"
	printStep(fmt.Sprintf("Phrase: '%s'", phrase))

	SendTestString(phrase, 30*time.Millisecond)
	time.Sleep(1 * time.Second)

	text := strings.TrimSpace(getEditText(uint32(editHwnd)))
	printStep(fmt.Sprintf("Result: '%s'", text))

	// Проверяем, что код и URL остались без изменений
	if !strings.Contains(text, "hello") {
		t.Errorf("'hello' was converted: '%s'", text)
	}
	if !strings.Contains(text, "world") {
		t.Errorf("'world' was converted: '%s'", text)
	}
	if !strings.Contains(text, "https://site.ru") {
		t.Errorf("'https://site.ru' was converted: '%s'", text)
	}

	// Проверяем, что русский текст переключился
	if !strings.Contains(text, "привет") {
		t.Errorf("'привет' was not converted: '%s'", text)
	}
	if !strings.Contains(text, "как") {
		t.Errorf("'как' was not converted: '%s'", text)
	}
}

// TestEnglishWordsNotConverted — проверка что английские слова не переключаются
func TestEnglishWordsNotConverted(t *testing.T) {
	printStep("Test: English words should NOT be converted")

	if err := SetTestMode(true); err != nil {
		t.Fatalf("SetTestMode: %v", err)
	}
	t.Cleanup(func() {
		SetTestMode(false)
	})

	_, editHwnd := launchTargetWindows()
	if editHwnd == 0 {
		t.Skip("Notepad not available (headless environment)")
	}

	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Только английские слова
	phrase := "hello world git docker kubernetes nginx"
	printStep(fmt.Sprintf("Phrase: '%s'", phrase))

	SendTestString(phrase, 30*time.Millisecond)
	time.Sleep(1 * time.Second)

	text := strings.TrimSpace(getEditText(uint32(editHwnd)))
	printStep(fmt.Sprintf("Result: '%s'", text))

	// Все слова должны остаться английскими
	expectedWords := []string{"hello", "world", "git", "docker", "kubernetes", "nginx"}
	for _, expected := range expectedWords {
		if !strings.Contains(text, expected) {
			t.Errorf("English word '%s' was converted: '%s'", expected, text)
		}
	}

	// Проверяем, что нет кириллицы
	if containsCyrillic(text) {
		t.Errorf("Text contains cyrillic (should be all English): '%s'", text)
	}
}

// ─────────────────────────────────────────────────────────────
// Helper Functions
// ─────────────────────────────────────────────────────────────

func containsCyrillic(s string) bool {
	for _, r := range s {
		if (r >= '\u0410' && r <= '\u044F') || r == '\u0401' || r == '\u0451' {
			return true
		}
	}
	return false
}
