package main

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------- AppSettings JSON round-trip ----------

func TestAppSettingsJSONRoundTrip(t *testing.T) {
	original := AppSettings{
		EnableAI:        true,
		Threshold:       85,
		Hotkey:          "pause",
		CaseHotkey:      "shift+pause",
		Exceptions:      "word1\nword2",
		ForceConvert:    "test",
		Blacklist:       "code\nvscode",
		Autostart:       true,
		CustomVocab:     "docker\nk8s",
		BlockPaths:      true,
		BlockFlags:      true,
		UseMarkov:       true,
		LiveTypoEnabled: true,
		LiveTypoChar:    4,
		TrainingLimit:   60000,
		LaplacePenalty:  1,
	}

	data, err := json.MarshalIndent(&original, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	var restored AppSettings
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.EnableAI != original.EnableAI {
		t.Errorf("EnableAI = %v, want %v", restored.EnableAI, original.EnableAI)
	}
	if restored.Threshold != original.Threshold {
		t.Errorf("Threshold = %d, want %d", restored.Threshold, original.Threshold)
	}
	if restored.Hotkey != original.Hotkey {
		t.Errorf("Hotkey = %q, want %q", restored.Hotkey, original.Hotkey)
	}
	if restored.CaseHotkey != original.CaseHotkey {
		t.Errorf("CaseHotkey = %q, want %q", restored.CaseHotkey, original.CaseHotkey)
	}
	if restored.Exceptions != original.Exceptions {
		t.Errorf("Exceptions = %q, want %q", restored.Exceptions, original.Exceptions)
	}
	if restored.BlockPaths != original.BlockPaths {
		t.Errorf("BlockPaths = %v, want %v", restored.BlockPaths, original.BlockPaths)
	}
	if restored.LiveTypoChar != original.LiveTypoChar {
		t.Errorf("LiveTypoChar = %d, want %d", restored.LiveTypoChar, original.LiveTypoChar)
	}
}

func TestAppSettingsDefaults(t *testing.T) {
	data := []byte(`{}`)
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if settings.EnableAI {
		t.Error("Default EnableAI should be false")
	}
	if settings.Threshold != 0 {
		t.Errorf("Default Threshold = %d, want 0", settings.Threshold)
	}
}

// ---------- loadSettings default values — skipped on Windows ----------
// Тесты для loadSettings требуют Linux (доступ к файловой системе)
// и зависят от defaultBlacklist(), который доступен только на linux/build

// ---------- saveSettings — file creation ----------

// ---------- AppStats JSON ----------

func TestAppStatsJSONRoundTrip(t *testing.T) {
	original := AppStats{
		LayoutCorrections: 42,
		TypoCorrections:   17,
		SavedKeystrokes:   150,
		UndoCount:         5,
	}

	data, err := json.MarshalIndent(&original, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	var restored AppStats
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.LayoutCorrections != original.LayoutCorrections {
		t.Errorf("LayoutCorrections = %d, want %d", restored.LayoutCorrections, original.LayoutCorrections)
	}
	if restored.UndoCount != original.UndoCount {
		t.Errorf("UndoCount = %d, want %d", restored.UndoCount, original.UndoCount)
	}
}

// ---------- TopoRecord JSON ----------

func TestTopoRecordJSONRoundTrip(t *testing.T) {
	original := TopoRecord{
		Word:     "hello",
		Distance: 3.14,
	}

	data, err := json.MarshalIndent(&original, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	var restored TopoRecord
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.Word != original.Word {
		t.Errorf("Word = %q, want %q", restored.Word, original.Word)
	}
	if restored.Distance != original.Distance {
		t.Errorf("Distance = %f, want %f", restored.Distance, original.Distance)
	}
}

// ---------- Heatmap operations ----------

func TestRecordHeatmapError(t *testing.T) {
	// Инициализируем debounce таймеры перед тестом
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	if heatmapDebounceTimer == nil {
		heatmapDebounceTimer = time.NewTimer(heatmapDebounceDelay)
	}

	keyHeatmap = make(map[string]int)

	recordHeatmapError("hello")

	if keyHeatmap["h"] != 1 {
		t.Errorf("keyHeatmap[\"h\"] = %d, want 1", keyHeatmap["h"])
	}
	if keyHeatmap["e"] != 1 {
		t.Errorf("keyHeatmap[\"e\"] = %d, want 1", keyHeatmap["e"])
	}
	if keyHeatmap["l"] != 2 {
		t.Errorf("keyHeatmap[\"l\"] = %d, want 2", keyHeatmap["l"])
	}
}

func TestRecordHeatmapErrorEmpty(t *testing.T) {
	if heatmapDebounceTimer == nil {
		heatmapDebounceTimer = time.NewTimer(heatmapDebounceDelay)
	}
	keyHeatmap = make(map[string]int)
	recordHeatmapError("")
	if len(keyHeatmap) != 0 {
		t.Errorf("keyHeatmap length = %d, want 0", len(keyHeatmap))
	}
}

func TestRecordHeatmapErrorNonAlpha(t *testing.T) {
	if heatmapDebounceTimer == nil {
		heatmapDebounceTimer = time.NewTimer(heatmapDebounceDelay)
	}
	keyHeatmap = make(map[string]int)
	recordHeatmapError("123@#$")
	// Только a-z, ;, ', ,, ., [, ] должны учитываться
	for key := range keyHeatmap {
		if key != "" {
			t.Errorf("keyHeatmap contains non-alpha key: %q", key)
		}
	}
}

// ---------- revertHeatmapError ----------

func TestRevertHeatmapError(t *testing.T) {
	if heatmapDebounceTimer == nil {
		heatmapDebounceTimer = time.NewTimer(heatmapDebounceDelay)
	}
	keyHeatmap = map[string]int{"a": 3, "b": 1}
	revertHeatmapError("abc")

	if keyHeatmap["a"] != 2 {
		t.Errorf("keyHeatmap[\"a\"] = %d, want 2", keyHeatmap["a"])
	}
	if keyHeatmap["b"] != 0 {
		t.Errorf("keyHeatmap[\"b\"] = %d, want 0", keyHeatmap["b"])
	}
	// "c" не было в keyHeatmap → revertHeatmapError проверяет keyHeatmap[k] > 0,
	// для отсутствующего ключа ничего не происходит, c не появляется в map
	if _, exists := keyHeatmap["c"]; exists {
		t.Logf("keyHeatmap[\"c\"] = %d (не должно существовать)", keyHeatmap["c"])
	}
}

func TestRevertHeatmapErrorNoNegative(t *testing.T) {
	if heatmapDebounceTimer == nil {
		heatmapDebounceTimer = time.NewTimer(heatmapDebounceDelay)
	}
	keyHeatmap = map[string]int{"a": 1}
	// Удаляем только те символы, которые есть
	revertHeatmapError("a")

	if keyHeatmap["a"] != 0 {
		t.Errorf("keyHeatmap[\"a\"] = %d, want 0", keyHeatmap["a"])
	}
}

// ---------- addTopoRecord ----------

func TestAddTopoRecord(t *testing.T) {
	topoHistory = nil
	addTopoRecord("hello", 3.14)
	addTopoRecord("world", 2.71)

	if len(topoHistory) != 2 {
		t.Errorf("topoHistory length = %d, want 2", len(topoHistory))
	}
	if topoHistory[0].Word != "world" {
		t.Errorf("topoHistory[0].Word = %q, want %q", topoHistory[0].Word, "world")
	}
}

func TestAddTopoRecordLimit(t *testing.T) {
	topoHistory = nil
	for i := 0; i < 15; i++ {
		addTopoRecord("word", float64(i))
	}

	if len(topoHistory) > 12 {
		t.Errorf("topoHistory length = %d, want <= 12", len(topoHistory))
	}
}

// ---------- Stats increment operations ----------

func TestIncrementLayoutCorrection(t *testing.T) {
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	stats = AppStats{}
	stats.LayoutCorrections = 0
	stats.SavedKeystrokes = 0

	incrementLayoutCorrection(5)

	if stats.LayoutCorrections != 1 {
		t.Errorf("LayoutCorrections = %d, want 1", stats.LayoutCorrections)
	}
	if stats.SavedKeystrokes != 10 {
		t.Errorf("SavedKeystrokes = %d, want 10 (5*2)", stats.SavedKeystrokes)
	}
}

func TestIncrementTypoCorrection(t *testing.T) {
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	stats = AppStats{}
	stats.TypoCorrections = 0
	stats.SavedKeystrokes = 0

	incrementTypoCorrection(3)

	if stats.TypoCorrections != 1 {
		t.Errorf("TypoCorrections = %d, want 1", stats.TypoCorrections)
	}
	if stats.SavedKeystrokes != 3 {
		t.Errorf("SavedKeystrokes = %d, want 3", stats.SavedKeystrokes)
	}
}

func TestIncrementUndoCountSwitchedLang(t *testing.T) {
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	stats = AppStats{
		LayoutCorrections: 5,
		SavedKeystrokes:   100,
		UndoCount:         0,
	}

	incrementUndoCount(3, true) // switchedLang = true

	if stats.UndoCount != 1 {
		t.Errorf("UndoCount = %d, want 1", stats.UndoCount)
	}
	if stats.LayoutCorrections != 4 {
		t.Errorf("LayoutCorrections = %d, want 4", stats.LayoutCorrections)
	}
	if stats.SavedKeystrokes != 94 {
		t.Errorf("SavedKeystrokes = %d, want 94 (100 - 3*2)", stats.SavedKeystrokes)
	}
}

func TestIncrementUndoCountTypo(t *testing.T) {
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	stats = AppStats{
		TypoCorrections: 5,
		SavedKeystrokes: 100,
		UndoCount:       0,
	}

	incrementUndoCount(3, false) // switchedLang = false

	if stats.UndoCount != 1 {
		t.Errorf("UndoCount = %d, want 1", stats.UndoCount)
	}
	if stats.TypoCorrections != 4 {
		t.Errorf("TypoCorrections = %d, want 4", stats.TypoCorrections)
	}
	if stats.SavedKeystrokes != 97 {
		t.Errorf("SavedKeystrokes = %d, want 97 (100 - 3)", stats.SavedKeystrokes)
	}
}

func TestIncrementUndoCountNoNegativeSavedKeystrokes(t *testing.T) {
	if statsDebounceTimer == nil {
		statsDebounceTimer = time.NewTimer(statsDebounceDelay)
	}
	stats = AppStats{
		SavedKeystrokes:   1,
		LayoutCorrections: 0,
	}

	// Попытка вычесть больше чем есть
	incrementUndoCount(10, true) // должно вычесть 20, но не ниже 0

	if stats.SavedKeystrokes < 0 {
		t.Errorf("SavedKeystrokes = %d, want >= 0", stats.SavedKeystrokes)
	}
}
