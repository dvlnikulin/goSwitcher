package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------- getNGrams ----------

func TestGetNGrams(t *testing.T) {
	tests := []struct {
		input       string
		expectedLen int
	}{
		{"hi", 6},   // ^h$, ^hi, hi$, ^hi + hi$ = 6 (2-grams: ^h ^i hi$, 3-grams: ^hi, 4-grams: ^hi$)
		{"hello", 15},
		{"a", 3},
		{"", 1}, // ^$ = 1 n-gram (n=2: i=0 to i<=0 → 1 element)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ngrams := getNGrams(tt.input)
			if len(ngrams) != tt.expectedLen {
				t.Errorf("getNGrams(%q) = %d ngrams, want %d", tt.input, len(ngrams), tt.expectedLen)
			}
			// Проверяем что все ngrams не пустые
			for _, ng := range ngrams {
				if len(ng) == 0 {
					t.Errorf("getNGrams(%q) contains empty ngram", tt.input)
				}
			}
		})
	}
}

// ---------- LanguageModel.Score ----------

func TestLanguageModelScoreNilModel(t *testing.T) {
	var lm *LanguageModel
	if score := lm.Score("test"); score != 0.0 {
		t.Errorf("nil LanguageModel.Score(\"test\") = %v, want 0", score)
	}
}

func TestLanguageModelScoreEmptyModel(t *testing.T) {
	lm := &LanguageModel{
		LogPriorPos:  -1.0,
		LogPriorNeg:  -1.0,
		LogProbPos:   make(map[string]float64),
		LogProbNeg:   make(map[string]float64),
		UnseenPos:    -5.0,
		UnseenNeg:    -5.0,
	}
	score := lm.Score("test")
	if score < 0 || score > 1 {
		t.Errorf("LanguageModel.Score(\"test\") = %v, want [0,1]", score)
	}
}

func TestLanguageModelScoreWithVocab(t *testing.T) {
	lm := &LanguageModel{
		LogPriorPos:  -0.5,
		LogPriorNeg:  -2.0,
		LogProbPos:   map[string]float64{"he": 0.1, "hel": 0.2, "ello": 0.3},
		LogProbNeg:   map[string]float64{},
		UnseenPos:    -3.0,
		UnseenNeg:    -5.0,
	}
	score := lm.Score("hello")
	if score < 0 || score > 1 {
		t.Errorf("LanguageModel.Score(\"hello\") = %v, want [0,1]", score)
	}
	// Поскольку положительная вероятность выше — score должен быть > 0.5
	if score < 0.5 {
		t.Errorf("LanguageModel.Score(\"hello\") = %v, want > 0.5", score)
	}
}

// ---------- sanitizeDictionary ----------

func TestSanitizeDictionary(t *testing.T) {
	vowels := "аеёиоуыэюяaeiouy"
	consonants := "бвгджзйклмнпрстфхцчшщbcdfghjklmnpqrstvwxz"

	tests := []struct {
		input  []string
		want   int
		reason string
	}{
		{[]string{"hello", "world"}, 2, "valid words"},
		{[]string{"bcdfg", "xyz"}, 1, "'bcdfg' has no vowels (len>=3, removed), 'xyz' has 'y' which is a vowel"},
		{[]string{"a", "ab", "abc"}, 2, "single chars removed (len<2), 'ab' ok for len<3, 'abc' has vowel 'a'"},
		{[]string{"rhythm"}, 1, "'y' is a vowel in 'aeiouy', so rhythm has vowel"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			result := sanitizeDictionary(tt.input, vowels, consonants)
			if len(result) != tt.want {
				t.Errorf("sanitizeDictionary(%v) = %d words, want %d", tt.input, len(result), tt.want)
			}
		})
	}
}

// ---------- loadWords ----------

func TestLoadWordsEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "words.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	words := loadWords(path)
	if len(words) != 0 {
		t.Errorf("loadWords(empty file) = %d words, want 0", len(words))
	}
}

func TestLoadWordsValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "words.txt")
	content := "hello\nworld\nhello\ntest\n"
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	words := loadWords(path)
	if len(words) != 3 {
		t.Errorf("loadWords() = %d words, want 3 (unique: hello, world, test)", len(words))
	}
	// Проверяем уникальность
	seen := make(map[string]bool)
	for _, w := range words {
		if seen[w] {
			t.Errorf("loadWords() contains duplicate: %q", w)
		}
		seen[w] = true
	}
}

func TestLoadWordsNonExistent(t *testing.T) {
	words := loadWords("/nonexistent/path/words.txt")
	if len(words) != 0 {
		t.Errorf("loadWords(nonexistent) = %d words, want 0", len(words))
	}
}

func TestLoadWordsWithBOM(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "words.txt")
	// UTF-8 BOM + слово
	content := "\xef\xbb\xbfпривет\nмир\n"
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	words := loadWords(path)
	if len(words) != 2 {
		t.Errorf("loadWords(BOM) = %d words, want 2", len(words))
	}
	if words[0] != "привет" {
		t.Errorf("loadWords(BOM) first word = %q, want %q", words[0], "привет")
	}
}

// ---------- IsImpossible ----------

func TestIsImpossible(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"bcdfg", true},   // 5 согласных подряд
		{"hello", false},
		{"rhythm", false}, // 'y' входит в гласные "aeiouy", значит hasVowel=true
		{"a", false},      // слишком короткое
		{"ab", false},     // слишком короткое
		{"bcdfghj", true}, // 7 согласных подряд
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsImpossible(tt.input)
			if result != tt.expected {
				t.Errorf("IsImpossible(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- isCyrillic ----------

func TestIsCyrillic(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"привет", true},
		{"Hello", false},
		{"hEllo", false},
		{"приветHello", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isCyrillic(tt.input)
			if result != tt.expected {
				t.Errorf("isCyrillic(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ---------- trainNaiveBayes ----------

func TestTrainNaiveBayes(t *testing.T) {
	positives := []string{"hello", "world", "test"}
	negatives := []string{"qwerty", "asdfgh", "zxcvbn"}

	lm := trainNaiveBayes(positives, negatives, 1.0)

	if lm == nil {
		t.Fatal("trainNaiveBayes returned nil")
	}
	if lm.LogProbPos == nil {
		t.Error("trainNaiveBayes: LogProbPos is nil")
	}
	if lm.LogProbNeg == nil {
		t.Error("trainNaiveBayes: LogProbNeg is nil")
	}
	// Score должен быть в диапазоне [0, 1]
	score := lm.Score("hello")
	if score < 0 || score > 1 {
		t.Errorf("LanguageModel.Score(\"hello\") = %v, want [0,1]", score)
	}
	// "hello" в положительных — score должен быть > 0.5
	if score < 0.5 {
		t.Errorf("LanguageModel.Score(\"hello\") = %v, want > 0.5", score)
	}
}

func TestTrainNaiveBayesEmptyInput(t *testing.T) {
	lm := trainNaiveBayes([]string{}, []string{}, 1.0)
	if lm == nil {
		t.Fatal("trainNaiveBayes returned nil")
	}
	// Score пустой модели — должен быть в диапазоне
	score := lm.Score("test")
	if score < 0 || score > 1 {
		t.Errorf("Empty LanguageModel.Score(\"test\") = %v, want [0,1]", score)
	}
}

// ---------- sanitizeDictionary — detailed ----------

func TestSanitizeDictionaryConsecutiveConsonants(t *testing.T) {
	// Слово с 5+ подряд согласных должно быть удалено
	input := []string{"strngth", "hello", "strgnth"}
	result := sanitizeDictionary(input, "аеёиоуыэюяaeiouy", "bcdfghjklmnpqrstvwxz")
	// "strngth" = 5 согласных подряд → удалено
	// "hello" = ok
	// "strgnth" = 5 согласных подряд → удалено
	if len(result) != 1 || result[0] != "hello" {
		t.Errorf("sanitizeDictionary = %v, want [hello]", result)
	}
}

// ---------- matchCase integration ----------

func TestMatchCasePreservesLength(t *testing.T) {
	// matchCase должен сохранять длину строки
	original := "HELLO"
	fixed := "the"
	result := matchCase(original, fixed)
	if len(result) != len(fixed) {
		t.Errorf("matchCase preserved length = %d, want %d", len(result), len(fixed))
	}
}

// ---------- getNGrams — edge cases ----------

func TestGetNGramsEdgeCases(t *testing.T) {
	// Пустое слово → marked = "^$", chars = ['^', '$'], 1 ngram (n=2: i=0 to i<=0)
	ngrams := getNGrams("")
	if len(ngrams) != 1 {
		t.Errorf("getNGrams(\"\") = %d ngrams, want 1", len(ngrams))
	}

	// Однобуквенное слово
	ngrams = getNGrams("a")
	if len(ngrams) == 0 {
		t.Error("getNGrams(\"a\") returned no ngrams")
	}
}

// ---------- Score boundary values ----------

func TestLanguageModelScoreBoundaryValues(t *testing.T) {
	lm := &LanguageModel{
		LogPriorPos: -10.0, // Очень низкий приоритет для positives
		LogPriorNeg: -0.1,
		LogProbPos:  map[string]float64{},
		LogProbNeg:  map[string]float64{},
		UnseenPos:   -10.0,
		UnseenNeg:   -0.1,
	}
	score := lm.Score("test")
	if score < 0 || score > 1 {
		t.Errorf("Score = %v, want [0,1]", score)
	}
	// При таких параметрах score должен быть близок к 0
	if score > 0.5 {
		t.Errorf("Score = %v with negative-biased model, want <= 0.5", score)
	}
}

// ---------- trainNaiveBayes — Laplace penalty ----------

func TestTrainNaiveBayesLaplacePenalty(t *testing.T) {
	positives := []string{"abc"}
	negatives := []string{"xyz"}

	// С penalty=0.001 (очень маленький)
	lm := trainNaiveBayes(positives, negatives, 0.001)
	score := lm.Score("abc")
	if score < 0 || score > 1 {
		t.Errorf("Score with small penalty = %v, want [0,1]", score)
	}

	// С penalty=10.0 (большой)
	lm2 := trainNaiveBayes(positives, negatives, 10.0)
	score2 := lm2.Score("abc")
	if score2 < 0 || score2 > 1 {
		t.Errorf("Score with large penalty = %v, want [0,1]", score2)
	}
}
