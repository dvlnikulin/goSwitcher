//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Test 1: Standard switching (ghbdtn + Space)
// ─────────────────────────────────────────────────────────────

func TestBasicSwitching(t *testing.T) {
	printStep("Test: basic switching ghbdtn + Space")

	// 1. Enable test mode (switcher ignores input)
	if err := SetTestMode(true); err != nil {
		t.Fatalf("SetTestMode: %v", err)
	}
	t.Cleanup(func() {
		SetTestMode(false)
		time.Sleep(100 * time.Millisecond)
	})

	// 2. Launch Notepad
	_, editHwnd := launchTargetWindows()
	if editHwnd == 0 {
		t.Skip("Notepad not available (headless environment)")
	}

	// 3. Type "ghbdtn" (this is "привет" in QWERTY)
	SendTestString("ghbdtn", 30*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// 4. Check: text should be "ghbdtn" (switcher disabled)
	text := getEditText(uint32(editHwnd))
	if text != "ghbdtn" {
		t.Errorf("After typing 'ghbdtn' with switcher off: '%s', expected 'ghbdtn'", text)
	}

	// 5. Enable switcher
	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 6. Clear and type again
	clearText(uint32(editHwnd))
	time.Sleep(100 * time.Millisecond)
	SendTestString("ghbdtn", 30*time.Millisecond)

	// 7. Press Space (should trigger switching)
	SendTestKey(VK_SPACE, 50*time.Millisecond)
	time.Sleep(AutoSwitchDelay)

	// 8. Check result
	text = strings.TrimSpace(getEditText(uint32(editHwnd)))
	expected := "привет "
	if text != expected {
		t.Errorf("After 'ghbdtn' + Space:\n  Got:      '%s'\n  Expected: '%s'", text, expected)
	}

	printStep(fmt.Sprintf("Result: '%s'", text))
}

// ─────────────────────────────────────────────────────────────
// Test 2: Stress Typing (fast typing, race conditions)
// ─────────────────────────────────────────────────────────────

func TestStressTyping(t *testing.T) {
	printStep("Test: stress typing (15ms per key)")

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

	phrase := "ghbdtn rfr ltkf xnj yjdjuj"
	printStep(fmt.Sprintf("Phrase: '%s'", phrase))

	// 1. Enable switcher
	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 2. Type fast (15ms per key)
	SendTestString(phrase, 15*time.Millisecond)

	// 3. Wait for processing
	time.Sleep(2 * time.Second)

	// 4. Check text
	text := strings.TrimSpace(getEditText(uint32(editHwnd)))

	// Expected: all words were switched
	expectedWords := []string{"привет", "как", "дела", "мир", "пока"}
	textLower := strings.ToLower(text)

	allFound := true
	for _, expected := range expectedWords {
		if !strings.Contains(textLower, expected) {
			allFound = false
			t.Logf("  Not found: '%s'", expected)
		}
	}

	if !allFound {
		t.Errorf("Stress typing did not switch all words:\n  Got:      '%s'\n  Expected contain: %v", text, expectedWords)
	}

	printStep(fmt.Sprintf("Result: '%s'", text))
}

// ─────────────────────────────────────────────────────────────
// Test 3: Pauses in the middle of a word
// ─────────────────────────────────────────────────────────────

func TestPauseInMiddle(t *testing.T) {
	printStep("Test: pause in middle of word")

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

	// 1. Enable switcher
	if err := SetTestMode(false); err != nil {
		t.Fatalf("SetTestMode off: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 2. Type "ghb"
	SendTestString("ghb", 30*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// 3. Pause 2 seconds
	printStep("Pause 2 seconds...")
	time.Sleep(2 * time.Second)

	// 4. Continue with "dtn" + Space
	SendTestString("dtn", 30*time.Millisecond)
	SendTestKey(VK_SPACE, 50*time.Millisecond)
	time.Sleep(AutoSwitchDelay)

	// 5. Check
	text := strings.TrimSpace(getEditText(uint32(editHwnd)))
	expected := "привет "

	if text != expected {
		t.Errorf("After 'ghb' + 2s pause + 'dtn' + Space:\n  Got:      '%s'\n  Expected: '%s'", text, expected)
	}

	printStep(fmt.Sprintf("Result: '%s'", text))
}
