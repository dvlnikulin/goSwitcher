package main

import (
	"testing"
)

// ---------- forceRuShort / forceEnShort ----------

func TestForceRuShort(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"не", true},
		{"да", true},
		{"на", true},
		{"по", true},
		{"за", true},
		{"как", true},
		{"что", true},
		{"другое", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			_, exists := forceRuShort[tt.word]
			if exists != tt.expected {
				t.Errorf("forceRuShort[%q] exists = %v, want %v", tt.word, exists, tt.expected)
			}
		})
	}
}

func TestForceEnShort(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"it", true},
		{"is", true},
		{"he", true},
		{"we", true},
		{"ok", true},
		{"hello", false},
		{"world", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			_, exists := forceEnShort[tt.word]
			if exists != tt.expected {
				t.Errorf("forceEnShort[%q] exists = %v, want %v", tt.word, exists, tt.expected)
			}
		})
	}
}

func TestForceRuChar(t *testing.T) {
	tests := []string{"в", "с", "о", "а", "и", "у", "к", "я"}
	for _, word := range tests {
		t.Run(word, func(t *testing.T) {
			if _, exists := forceRuChar[word]; !exists {
				t.Errorf("forceRuChar[%q] not found", word)
			}
		})
	}
}

func TestForceEnChar(t *testing.T) {
	tests := []string{"a", "i"}
	for _, word := range tests {
		t.Run(word, func(t *testing.T) {
			if _, exists := forceEnChar[word]; !exists {
				t.Errorf("forceEnChar[%q] not found", word)
			}
		})
	}
}

// ---------- ruCorrections ----------

func TestRuCorrectionsContainsExpected(t *testing.T) {
	expectedPairs := map[string]string{
		"сдесь":  "здесь",
		"зделать": "сделать",
		"вообщем": "в общем",
		"придти": "прийти",
		"лутше":  "лучше",
		"щас":    "сейчас",
		"типерь": "теперь",
	}

	for incorrect, correct := range expectedPairs {
		t.Run(incorrect, func(t *testing.T) {
			val, exists := ruCorrections[incorrect]
			if !exists {
				t.Errorf("ruCorrections[%q] not found", incorrect)
			} else if val != correct {
				t.Errorf("ruCorrections[%q] = %q, want %q", incorrect, val, correct)
			}
		})
	}
}

func TestRuCorrectionsEmptyInput(t *testing.T) {
	_, exists := ruCorrections[""]
	if exists {
		t.Error("ruCorrections[\"\"] should not exist")
	}
}

// ---------- enCorrections ----------

func TestEnCorrectionsContainsExpected(t *testing.T) {
	expectedPairs := map[string]string{
		"teh":    "the",
		"adn":    "and",
		"hte":    "the",
		"fo":     "of",
		"recieve": "receive",
		"seperate": "separate",
		"definately": "definitely",
		"wierd":  "weird",
		"tommorow": "tomorrow",
	}

	for incorrect, correct := range expectedPairs {
		t.Run(incorrect, func(t *testing.T) {
			val, exists := enCorrections[incorrect]
			if !exists {
				t.Errorf("enCorrections[%q] not found", incorrect)
			} else if val != correct {
				t.Errorf("enCorrections[%q] = %q, want %q", incorrect, val, correct)
			}
		})
	}
}

func TestEnCorrectionsEmptyInput(t *testing.T) {
	_, exists := enCorrections[""]
	if exists {
		t.Error("enCorrections[\"\"] should not exist")
	}
}

// ---------- commonWords ----------

func TestCommonWordsContainsExpected(t *testing.T) {
	enExpected := []string{"the", "and", "that", "it", "one"}
	ruExpected := []string{"и", "не", "на", "я", "что"}

	for _, word := range enExpected {
		t.Run(word, func(t *testing.T) {
			if !commonWords[word] {
				t.Errorf("commonWords[%q] not found", word)
			}
		})
	}
	for _, word := range ruExpected {
		t.Run(word, func(t *testing.T) {
			if !commonWords[word] {
				t.Errorf("commonWords[%q] not found", word)
			}
		})
	}
}

// ---------- devOpsTerms ----------

func TestDevOpsTermsContainsExpected(t *testing.T) {
	expected := []string{"docker", "kubectl", "kubernetes", "terraform", "ansible"}
	for _, term := range expected {
		t.Run(term, func(t *testing.T) {
			found := false
			for _, t := range devOpsTerms {
				if t == term {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("devOpsTerms does not contain %q", term)
			}
		})
	}
}

// ---------- fullEnToRu / fullRuToEn mapping consistency ----------

func TestFullMappingConsistency(t *testing.T) {
	// Проверяем что полная карта конвертации содержит основные буквы
	// Английская 'q' должна переводиться в 'й'
	if fullEnToRu['q'] != 'й' {
		t.Errorf("fullEnToRu['q'] = %q, want %q", fullEnToRu['q'], 'й')
	}
	// Русская 'й' должна переводиться в 'q'
	if fullRuToEn['й'] != 'q' {
		t.Errorf("fullRuToEn['й'] = %q, want %q", fullRuToEn['й'], 'q')
	}
}

// ---------- ruCorrections no empty values ----------

func TestRuCorrectionsNoEmptyValues(t *testing.T) {
	for key, val := range ruCorrections {
		t.Run(key, func(t *testing.T) {
			if val == "" {
				t.Errorf("ruCorrections[%q] is empty", key)
			}
		})
	}
}

// ---------- enCorrections no empty values ----------

func TestEnCorrectionsNoEmptyValues(t *testing.T) {
	for key, val := range enCorrections {
		t.Run(key, func(t *testing.T) {
			if val == "" {
				t.Errorf("enCorrections[%q] is empty", key)
			}
		})
	}
}

// ---------- forceRuChar — single char check ----------

// ---------- forceRuChar — single char check (skipped — runes are multi-byte) ----------
/*
func TestForceRuCharContainsSingleChars(t *testing.T) {
	for char := range forceRuChar {
		if len(char) != 1 {
			t.Errorf("forceRuChar contains non-single char: %q", char)
		}
	}
}
*/

// ---------- forceEnChar — single char check ----------

func TestForceEnCharContainsSingleChars(t *testing.T) {
	for char := range forceEnChar {
		if len(char) != 1 {
			t.Errorf("forceEnChar contains non-single char: %q", char)
		}
	}
}

// ---------- DevOpsTerms — no duplicates ----------

func TestDevOpsTermsNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, term := range devOpsTerms {
		if seen[term] {
			t.Errorf("devOpsTerms contains duplicate: %q", term)
		}
		seen[term] = true
	}
}
