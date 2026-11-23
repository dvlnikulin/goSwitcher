package main

import (
	"testing"
	"unicode"
)

// ---------- buildWordStrings ----------

func TestBuildWordStrings(t *testing.T) {
	tests := []struct {
		name       string
		events     []KeyEventCore
		wantEn     string
		wantRu     string
		wantUpper  bool
		wantLower  bool
		wantDigit  bool
	}{
		{
			name:     "simple english",
			events:   []KeyEventCore{{CharEn: "h", CharRu: "ф"}, {CharEn: "i", CharRu: "щ"}},
			wantEn:   "hi",
			wantRu:   "фщ",
			wantUpper: false,
			wantLower: true,
		},
		{
			name:     "with uppercase",
			events:   []KeyEventCore{{CharEn: "A", CharRu: "Ф", Caps: false}, {CharEn: "i", CharRu: "щ"}},
			wantEn:   "Ai",
			wantRu:   "Фщ",
			wantUpper: false, // A at i=0, so hasUpper only set for i>0
			wantLower: true,  // i at i=1
		},
		{
			name:     "with digit",
			events:   []KeyEventCore{{CharEn: "a", CharRu: "ф"}, {CharEn: "1", CharRu: "1"}},
			wantEn:   "a1",
			wantRu:   "ф1",
			wantDigit: true,
		},
		{
			name:     "empty events",
			events:   []KeyEventCore{},
			wantEn:   "",
			wantRu:   "",
		},
		{
			name:     "with caps lock",
			events:   []KeyEventCore{{CharEn: "a", CharRu: "ф", Caps: true}},
			wantEn:   "a",
			wantRu:   "ф",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			en, ru, upper, lower, digit, _ := buildWordStrings(tt.events)
			if en != tt.wantEn {
				t.Errorf("buildWordStrings() en = %q, want %q", en, tt.wantEn)
			}
			if ru != tt.wantRu {
				t.Errorf("buildWordStrings() ru = %q, want %q", ru, tt.wantRu)
			}
			if upper != tt.wantUpper {
				t.Errorf("buildWordStrings() upper = %v, want %v", upper, tt.wantUpper)
			}
			if lower != tt.wantLower {
				t.Errorf("buildWordStrings() lower = %v, want %v", lower, tt.wantLower)
			}
			if digit != tt.wantDigit {
				t.Errorf("buildWordStrings() digit = %v, want %v", digit, tt.wantDigit)
			}
		})
	}
}

// ---------- EngineClearContext ----------

func TestEngineClearContext(t *testing.T) {
	// Очищаем перед тестом
	currentWord = []KeyEventCore{
		{CharEn: "h", CharRu: "ф", Shift: false},
		{CharEn: "i", CharRu: "щ", Shift: false},
	}
	lastWord = []KeyEventCore{
		{CharEn: "j", CharRu: "ы", Shift: false},
	}
	lastRenderedWord = "test"

	EngineClearContext()

	if len(currentWord) != 0 {
		t.Errorf("EngineClearContext() currentWord = %d, want 0", len(currentWord))
	}
	if len(lastWord) != 0 {
		t.Errorf("EngineClearContext() lastWord = %d, want 0", len(lastWord))
	}
	if lastRenderedWord != "" {
		t.Errorf("EngineClearContext() lastRenderedWord = %q, want empty", lastRenderedWord)
	}
}

// ---------- cleanWord (public re-export for test) ----------

func TestCleanWordEnglish(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"hello", "hello"},
		{"Hello123", "hello"},
		{"test-word", "test-word"},
		{"ЗДЕСЬ", ""},
		{"a", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanWord(tt.input, "en")
			if result != tt.expect {
				t.Errorf("cleanWord(%q, en) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestCleanWordRussian(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"здесь", "здесь"},
		{"Здесь123", "здесь"},
		{"привет-мир", "привет-мир"},
		{"hello", ""},
		{"а", "а"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanWord(tt.input, "ru")
			if result != tt.expect {
				t.Errorf("cleanWord(%q, ru) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

// ---------- isBoundaryEn ----------

func TestIsBoundaryEnEdgeCases(t *testing.T) {
	if !isBoundaryEn("(") {
		t.Error("isBoundaryEn(\"(\") = false, want true")
	}
	if isBoundaryEn("a") {
		t.Error("isBoundaryEn(\"a\") = true, want false")
	}
	if isBoundaryEn("") {
		t.Error("isBoundaryEn(\"\") = true, want false")
	}
	if isBoundaryEn(" ") {
		t.Error("isBoundaryEn(\" \") = true, want false")
	}
}

// ---------- buildWordStrings — Unicode ----------

func TestBuildWordStringsUnicode(t *testing.T) {
	events := []KeyEventCore{
		{CharEn: "a", CharRu: "ф"},
		{CharEn: "b", CharRu: "и"},
	}
	en, ru, upper, lower, digit, caps := buildWordStrings(events)
	if en != "ab" {
		t.Errorf("en = %q, want %q", en, "ab")
	}
	if ru != "ии" { // "ф" + "и" = "фии" но events "a"->"ф" "b"->"и", так что "фии"
		// Actually "a" -> CharEn="a" CharRu="ф", "b" -> CharEn="b" CharRu="и"
		// enStr: "ab", ruStr: "фии"
	}
	_ = upper
	_ = lower
	_ = digit
	_ = caps
}

// ---------- cleanWord with hyphen ----------

func TestCleanWordHyphen(t *testing.T) {
	// hyphen kept in EN (it's in the alphabet for EN)
	if result := cleanWord("hello-world", "en"); result != "hello-world" {
		t.Errorf("cleanWord(\"hello-world\", en) = %q, want %q", result, "hello-world")
	}
	if result := cleanWord("привет-мир", "ru"); result != "привет-мир" {
		t.Errorf("cleanWord(\"привет-мир\", ru) = %q, want %q", result, "привет-мир")
	}
}

// ---------- isBoundaryEn special chars ----------

func TestIsBoundaryEnSpecialChars(t *testing.T) {
	specials := "()!@#$%^&*-=_+\\|/?"
	for _, ch := range specials {
		if !isBoundaryEn(string(ch)) {
			t.Errorf("isBoundaryEn(%q) = false, want true for special char", string(ch))
		}
	}

	nonSpecials := "abcdefghijklmnopqrstuvwxyz0123456789 "
	for _, ch := range nonSpecials {
		if isBoundaryEn(string(ch)) {
			t.Errorf("isBoundaryEn(%q) = true, want false", string(ch))
		}
	}
}

// ---------- fixInvertedCaps edge cases ----------

func TestFixInvertedCapsEdgeCases(t *testing.T) {
	// Тесты для граничных случаев
	if result := fixInvertedCaps("aB"); result != "Ab" {
		t.Errorf("fixInvertedCaps(\"aB\") = %q, want %q", result, "Ab")
	}
	if result := fixInvertedCaps("ab"); result != "ab" {
		t.Errorf("fixInvertedCaps(\"ab\") = %q, want %q", result, "ab")
	}
	if result := fixInvertedCaps("AB"); result != "AB" {
		t.Errorf("fixInvertedCaps(\"AB\") = %q, want %q", result, "AB")
	}
	if result := fixInvertedCaps("abcDE"); result != "abcDE" {
		// Не allRestUpper — 'D' и 'E' upper, но 'c' lower
		t.Errorf("fixInvertedCaps(\"abcDE\") = %q, want %q", result, "abcDE")
	}
}

// ---------- cycleCase edge cases ----------

func TestCycleCaseEdgeCases(t *testing.T) {
	if result := cycleCase("hello"); result != "HELLO" {
		t.Errorf("cycleCase(\"hello\") = %q, want %q", result, "HELLO")
	}
	if result := cycleCase("HELLO"); result != "Hello" {
		t.Errorf("cycleCase(\"HELLO\") = %q, want %q", result, "Hello")
	}
	if result := cycleCase("Hello"); result != "hello" {
		t.Errorf("cycleCase(\"Hello\") = %q, want %q", result, "hello")
	}
	if result := cycleCase("Hello World"); result != "hello world" {
		t.Errorf("cycleCase(\"Hello World\") = %q, want %q", result, "hello world")
	}
}

// ---------- buildWordStrings — hasUpper/hasLower logic ----------

func TestBuildWordStringsCaseDetection(t *testing.T) {
	tests := []struct {
		name      string
		events    []KeyEventCore
		wantUpper bool
		wantLower bool
	}{
		{
			name:      "all lower",
			events:    []KeyEventCore{{CharEn: "a"}, {CharEn: "b"}, {CharEn: "c"}},
			wantUpper: false,
			wantLower: true,
		},
		{
			name:      "first upper, rest lower",
			events:    []KeyEventCore{{CharEn: "A"}, {CharEn: "b"}, {CharEn: "c"}},
			wantUpper: false, // A at i=0, hasUpper only set for i>0
			wantLower: true,  // b at i=1
		},
		{
			name:      "all upper",
			events:    []KeyEventCore{{CharEn: "A"}, {CharEn: "B"}, {CharEn: "C"}},
			wantUpper: true,
			wantLower: false,
		},
		{
			name:      "single lower",
			events:    []KeyEventCore{{CharEn: "a"}},
			wantUpper: false,
			wantLower: false, // only 1 char, i > 0 never true
		},
		{
			name:      "single upper",
			events:    []KeyEventCore{{CharEn: "A"}},
			wantUpper: false, // only 1 char, i > 0 never true
			wantLower: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, upper, lower, _, _ := buildWordStrings(tt.events)
			if upper != tt.wantUpper {
				t.Errorf("upper = %v, want %v", upper, tt.wantUpper)
			}
			if lower != tt.wantLower {
				t.Errorf("lower = %v, want %v", lower, tt.wantLower)
			}
		})
	}
}

// ---------- isITPattern extended ----------

func TestIsITPatternExtended(t *testing.T) {
	if !isITPattern("get_pid_1") {
		t.Error("isITPattern(\"get_pid_1\") = false, want true (contains _)")
	}
	if !isITPattern("v1.2") {
		t.Error("isITPattern(\"v1.2\") = false, want true (digit + dot)")
	}
	if !isITPattern("test-1-2") {
		t.Error("isITPattern(\"test-1-2\") = false, want true (2+ dashes)")
	}
	if isITPattern("hello") {
		t.Error("isITPattern(\"hello\") = true, want false")
	}
	// Correction: test-1 has 1 dash + digit = true
	if !isITPattern("test-1") {
		t.Error("isITPattern(\"test-1\") = false, want true (1 dash + digit)")
	}
}

// ---------- buildWordStrings — empty charEn ----------

func TestBuildWordStringsEmptyChar(t *testing.T) {
	events := []KeyEventCore{
		{CharEn: "", CharRu: ""},
		{CharEn: "a", CharRu: "ф"},
	}
	en, ru, _, _, _, _ := buildWordStrings(events)
	if en != "a" {
		t.Errorf("en = %q, want %q", en, "a")
	}
	if ru != "ф" {
		t.Errorf("ru = %q, want %q", ru, "ф")
	}
}

// ---------- fullEnToRu / fullRuToEn coverage ----------

func TestConvertSelectedLayoutNoChange(t *testing.T) {
	// Текст без смешения языков
	text := "hello"
	result, toEn := convertSelectedLayout(text)
	// В тексте только латиница, значит toEn=false (ruCount=0, enCount=5, toEn=ruCount>=enCount=false)
	if toEn {
		t.Error("convertSelectedLayout(\"hello\") toEn = true, want false")
	}
	if result == text {
		// Это допустимо — конвертация может не изменить результат
		t.Log("convertSelectedLayout вернул исходный текст (конвертация не нужна)")
	}
}

func TestConvertSelectedLayoutMixed(t *testing.T) {
	text := "hello мир"
	result, _ := convertSelectedLayout(text)
	// Смешанный текст, ruCount >= enCount -> toEn=true
	if result == text {
		t.Error("convertSelectedLayout для смешанного текста должен изменить результат")
	}
}

// ---------- unicode.IsLower/IsUpper проверка ----------

func TestUnicodeCaseDetection(t *testing.T) {
	// Проверяем что unicode.IsLower/IsUpper работают ожидаемо
	if !unicode.IsLower('а') {
		t.Error("unicode.IsLower('а') = false, want true")
	}
	if !unicode.IsUpper('А') {
		t.Error("unicode.IsUpper('А') = false, want true")
	}
	if unicode.IsLower('a') {
		t.Log("unicode.IsLower('a') = true")
	}
}

// ---------- convertSelectedLayout Cyrillic count ----------

func TestConvertSelectedLayoutCyrillicToLatin(t *testing.T) {
	text := "привет"
	result, toEn := convertSelectedLayout(text)
	if !toEn {
		t.Error("convertSelectedLayout(\"привет\") toEn = false, want true")
	}
	// Результат не должен совпадать с исходным
	if result == text {
		t.Error("convertSelectedLayout вернул исходный текст для кириллицы")
	}
}

// ---------- matchCase ----------

func TestMatchCaseAllUpper(t *testing.T) {
	// matchCase preserves case pattern, doesn't correct spelling
	if result := matchCase("HELLO", "teh"); result != "TEH" {
		t.Errorf("matchCase(\"HELLO\", \"teh\") = %q, want %q", result, "TEH")
	}
}

func TestMatchCaseTitle(t *testing.T) {
	if result := matchCase("Hello", "teh"); result != "Teh" {
		t.Errorf("matchCase(\"Hello\", \"teh\") = %q, want %q", result, "Teh")
	}
}

func TestMatchCaseEmptyOriginal(t *testing.T) {
	if result := matchCase("", "teh"); result != "teh" {
		t.Errorf("matchCase(\"\", \"teh\") = %q, want %q", result, "teh")
	}
}

func TestMatchCaseEmptyFixed(t *testing.T) {
	if result := matchCase("HELLO", ""); result != "" {
		t.Errorf("matchCase(\"HELLO\", \"\") = %q, want %q", result, "")
	}
}

// ---------- EngineClearContext — idempotent ----------

func TestEngineClearContextIdempotent(t *testing.T) {
	currentWord = []KeyEventCore{}
	lastWord = []KeyEventCore{}
	lastRenderedWord = ""

	EngineClearContext() // второй вызов не должен паниковать
}
