//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const switcherAPI = "http://127.0.0.1:8765"

// ─────────────────────────────────────────────────────────────
// Test mode API helpers
// ─────────────────────────────────────────────────────────────

// SetTestMode — включить/выключить тестовый режим свитчера
func SetTestMode(enabled bool) error {
	payload, _ := json.Marshal(map[string]bool{"enabled": enabled})
	resp, err := http.Post(switcherAPI+"/api/test_mode", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("SetTestMode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SetTestMode: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("SetTestMode decode: %w", err)
	}

	fmt.Printf("  [API] Test mode: %v\n", result["test_mode"])
	return nil
}

// GetTestMode — получить текущее состояние тестового режима
func GetTestMode() (bool, error) {
	resp, err := http.Get(switcherAPI + "/api/test_mode")
	if err != nil {
		return false, fmt.Errorf("GetTestMode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GetTestMode: status=%d", resp.StatusCode)
	}

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("GetTestMode decode: %w", err)
	}

	return result["test_mode"], nil
}

// ─────────────────────────────────────────────────────────────
// Settings API helpers
// ─────────────────────────────────────────────────────────────

// SetSwitcherSettings — изменить настройки свитчера через API
func SetSwitcherSettings(settings map[string]interface{}) error {
	payload, _ := json.Marshal(settings)
	resp, err := http.Post(switcherAPI+"/api/settings", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("SetSwitcherSettings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SetSwitcherSettings: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// GetSwitcherSettings — получить текущие настройки
func GetSwitcherSettings() (map[string]interface{}, error) {
	resp, err := http.Get(switcherAPI + "/api/settings")
	if err != nil {
		return nil, fmt.Errorf("GetSwitcherSettings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetSwitcherSettings: status=%d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("GetSwitcherSettings decode: %w", err)
	}

	return result, nil
}

// ─────────────────────────────────────────────────────────────
// Stats API helpers
// ─────────────────────────────────────────────────────────────

// GetStats — получить статистику
func GetStats() (map[string]interface{}, error) {
	resp, err := http.Get(switcherAPI + "/api/stats")
	if err != nil {
		return nil, fmt.Errorf("GetStats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetStats: status=%d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("GetStats decode: %w", err)
	}

	return result, nil
}

// ─────────────────────────────────────────────────────────────
// Test configuration — быстрая установка настроек для E2E
// ─────────────────────────────────────────────────────────────

// ConfigureForE2E — оптимальные настройки для E2E-тестов
func ConfigureForE2E() error {
	// Включаем AI, снижаем порог для надёжного переключения
	settings := map[string]interface{}{
		"enable_ai":        true,
		"threshold":        50, // ниже = легче переключается
		"hotkey":           "pause",
		"case_hotkey":      "shift+pause",
		"blacklist":        "", // отключаем blacklist для тестов
		"block_paths":      false,
		"block_flags":      false,
		"live_typo_enabled": false, // отключаем live-typo для предсказуемости
	}

	return SetSwitcherSettings(settings)
}

// ─────────────────────────────────────────────────────────────
// Retry helpers
// ─────────────────────────────────────────────────────────────

// retryWithDelay выполняет функцию с повторными попытками
func retryWithDelay(fn func() error, maxRetries int, delay time.Duration) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// waitUntil ожидает выполнения условия с таймаутом
func waitUntil(fn func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
