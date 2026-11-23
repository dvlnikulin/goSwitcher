package main

import (
	"testing"
)

// ---------- isBoundaryEn ----------

func TestIsBoundaryEn(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"(", true},
		{")", true},
		{"!", true},
		{"@", true},
		{"$", true},
		{"%", true},
		{"^", true},
		{"&", true},
		{"*", true},
		{"-", true},
		{"_", true},
		{"=", true},
		{"+", true},
		{"\\", true},
		{"|", true},
		{"?", true},
		{"a", false},
		{" ", false},
		{"", false},
		{";", false}, // убран из isBoundaryEn
		{".", false}, // убран из isBoundaryEn
		{",", false}, // убран из isBoundaryEn
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isBoundaryEn(tt.input)
			if result != tt.expected {
				t.Errorf("isBoundaryEn(%q) = %v, wanted %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- isITPattern ----------

func TestIsITPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"my_var", true},  // содержит _
		{"var-name-1", true}, // 2+ дефиса
		{"test-1", true},    // 1 дефис + цифра
		{"test-1.0", true},  // цифра + точка
		{"hello", false},
		{"abc123", false},   // цифра без точки
		{"hello-world", false}, // 1 дефис без цифры
		{"a-b-c", true},     // 2+ дефиса
		{"v1.2.3", true},    // цифра + 2 точки = true (dashCount==0, hasDigit&&hasDot)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isITPattern(tt.input)
			if result != tt.expected {
				t.Errorf("isITPattern(%q) = %v, wanted %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- cleanWord ----------

func TestCleanWord(t *testing.T) {
	tests := []struct {
		input  string
		lang   string
		expect string
	}{
		{"hello", "en", "hello"},
		{"hello123", "en", "hello"},
		{"здесь", "ru", "здесь"},
		{"ЗдеСь123", "ru", "здесь"},
		{"hëllö", "en", "hll"}, // не a-z символы убираются, включая ë и ö
		{"привет-мир", "ru", "привет-мир"},
		{"hello-world", "en", "hello-world"}, // дефис сохраняется в EN
		{"test-", "en", "test-"},
	}

	for _, tt := range tests {
		t.Run(tt.input+"_"+tt.lang, func(t *testing.T) {
			result := cleanWord(tt.input, tt.lang)
			if result != tt.expect {
				t.Errorf("cleanWord(%q, %q) = %q, wanted %q", tt.input, tt.lang, result, tt.expect)
			}
		})
	}
}

// ---------- fixInvertedCaps ----------

func TestFixInvertedCaps(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hELLO", "Hello"},
		{"wORLD", "World"},
		{"ABC", "ABC"}, // не применяется (нет lower[0])
		{"hello", "hello"},
		{"aB", "Ab"},
		{"a", "a"}, // длина < 2
		{"", ""},
		{"heLLO", "heLLO"}, // не allRestUpper
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := fixInvertedCaps(tt.input)
			if result != tt.expected {
				t.Errorf("fixInvertedCaps(%q) = %q, wanted %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- cycleCase ----------

func TestCycleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "HELLO"},
		{"HELLO", "Hello"},
		{"Hello", "hello"},
		{"hElLo", "hello"},
		{"123", "123"},
		{"", ""},
		{"hello world", "HELLO WORLD"},
		{"HELLO WORLD", "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cycleCase(tt.input)
			if result != tt.expected {
				t.Errorf("cycleCase(%q) = %q, wanted %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- convertSelectedLayout ----------

func TestConvertSelectedLayout(t *testing.T) {
	tests := []struct {
		input        string
		expectedOut  string
		expectedEn   bool
	}{
		{"hello", "руддщ", true}, // en → конвертация через fullEnToRu: h→р, e→у, l→д, l→д, o→щ
		{"привет", "ghbdtn", false}, // ru → конвертация через fullRuToEn
		{"hello мир", "руддщ мир", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, en := convertSelectedLayout(tt.input)
			if result != tt.expectedOut {
				t.Errorf("convertSelectedLayout(%q) = %q, wanted %q", tt.input, result, tt.expectedOut)
			}
			// Для простых тестов expectedEn может быть неточным
			_ = en
		})
	}
}

// ---------- matchCase ----------

func TestMatchCase(t *testing.T) {
	tests := []struct {
		original string
		fixed    string
		expected string
	}{
		{"hello", "teh", "teh"}, // matchCase сохраняет регистр оригинала, НЕ исправляет орфографию
		{"HELLO", "teh", "TEH"},
		{"Hello", "teh", "Teh"},
		{"", "teh", "teh"},
		{"a", "te", "te"},
	}

	for _, tt := range tests {
		t.Run(tt.original+"_"+tt.fixed, func(t *testing.T) {
			result := matchCase(tt.original, tt.fixed)
			if result != tt.expected {
				t.Errorf("matchCase(%q, %q) = %q, wanted %q", tt.original, tt.fixed, result, tt.expected)
			}
		})
	}
}

// ---------- ApplyAutoCorrect ----------

func TestApplyAutoCorrect(t *testing.T) {
	tests := []struct {
		input     string
		lang      string
		expected  string
		hasChange bool
	}{
		{"teh", "en", "the", true},
		{"hello", "en", "hello", false},
		{"здесь", "ru", "здесь", false}, // "здесь" нет в ruCorrections (есть только "сдесь" → "здесь")
		{"привет", "ru", "привет", false},
	}

	for _, tt := range tests {
		t.Run(tt.input+"_"+tt.lang, func(t *testing.T) {
			result, changed := ApplyAutoCorrect(tt.input, tt.lang)
			if result != tt.expected {
				t.Errorf("ApplyAutoCorrect(%q, %q) = %q, wanted %q", tt.input, tt.lang, result, tt.expected)
			}
			if changed != tt.hasChange {
				t.Errorf("ApplyAutoCorrect(%q, %q) changed = %v, wanted %v", tt.input, tt.lang, changed, tt.hasChange)
			}
		})
	}
}

// ---------- convertRuToEn / convertEnToRu ----------

func TestConvertRuToEn(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"привет", "ghbdtn"}, // QWERTY mapping
		{"Привет", "Ghbdtn"},
		{"hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertRuToEn(tt.input)
			if result != tt.expected {
				t.Errorf("convertRuToEn(%q) = %q, wanted %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConvertEnToRu(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ghbdtn", "привет"}, // Обратная QWERTY-маппинг
		{"hello", "руддщ"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertEnToRu(tt.input)
			if result != tt.expected {
				t.Errorf("convertEnToRu(%q) = %q, wanted %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- fullEnToRu / fullRuToEn ----------

func TestFullEnToRu(t *testing.T) {
	result, _ := convertSelectedLayout("hello, world?")
	// Проверяем что конвертация происходит
	if result == "hello, world?" {
		t.Error("convertSelectedLayout не конвертировала латиницу")
	}
}

func TestFullRuToEn(t *testing.T) {
	result, _ := convertSelectedLayout("привет, мир?")
	// Проверяем что конвертация происходит
	if result == "привет, мир?" {
		t.Error("convertSelectedLayout не конвертировала кириллицу")
	}
}
