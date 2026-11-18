package main

import (
	"strings"
	"unicode"
)

var fullEnToRu = map[rune]rune{
	'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш', 'o': 'щ', 'p': 'з', '[': 'х', ']': 'ъ',
	'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л', 'l': 'д', ';': 'ж', '\'': 'э',
	'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь', ',': 'б', '.': 'ю', '`': 'ё',
	'Q': 'Й', 'W': 'Ц', 'E': 'У', 'R': 'К', 'T': 'Е', 'Y': 'Н', 'U': 'Г', 'I': 'Ш', 'O': 'Щ', 'P': 'З', '{': 'Х', '}': 'Ъ',
	'A': 'Ф', 'S': 'Ы', 'D': 'В', 'F': 'А', 'G': 'П', 'H': 'Р', 'J': 'О', 'K': 'Л', 'L': 'Д', ':': 'Ж', '"': 'Э',
	'Z': 'Я', 'X': 'Ч', 'C': 'С', 'V': 'М', 'B': 'И', 'N': 'Т', 'M': 'Ь', '<': 'Б', '>': 'Ю', '~': 'Ё',
	'@': '"', '#': '№', '$': ';', '^': ':', '&': '?', '/': '.', '?': ',', '|': '/',
}

var fullRuToEn = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`',
	'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
	'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J', 'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
	'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M', 'Б': '<', 'Ю': '>', 'Ё': '~',
	'"': '@', '№': '#', ';': '$', ':': '^', '?': '&', ',': '?',
}

func isBoundaryEn(charEn string) bool {
	// ⚡ Убрали [ ] ; ' , . ` — теперь программа не ломает русские слова об английскую пунктуацию
	return strings.ContainsAny(charEn, "()!@#$%^&*-=_+\\|/?")
}

func isITPattern(wordEn string) bool {
	if strings.Contains(wordEn, "_") {
		return true
	}
	hasDigit, hasDot := false, false
	dashCount := strings.Count(wordEn, "-")

	for _, r := range wordEn {
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == '.' {
			hasDot = true
		}
	}
	if dashCount > 1 {
		return true
	}
	if dashCount == 1 && hasDigit {
		return true
	}
	return hasDigit && hasDot
}

// ⚡ Теперь очистка слова понимает, какой язык мы чистим
func cleanWord(word string, lang string) string {
	var res strings.Builder
	for _, r := range word {
		r = unicode.ToLower(r)
		if lang == "ru" {
			if unicode.Is(unicode.Cyrillic, r) || r == '-' {
				res.WriteRune(r)
			}
		} else {
			if (r >= 'a' && r <= 'z') || r == '-' {
				res.WriteRune(r)
			}
		}
	}
	return res.String()
}

func fixInvertedCaps(word string) string {
	runes := []rune(word)
	if len(runes) < 2 {
		return word
	}
	if unicode.IsLower(runes[0]) {
		allRestUpper, hasLetters := true, false
		for i := 1; i < len(runes); i++ {
			if unicode.IsLetter(runes[i]) {
				hasLetters = true
				if !unicode.IsUpper(runes[i]) {
					allRestUpper = false
					break
				}
			}
		}
		if hasLetters && allRestUpper {
			res := make([]rune, len(runes))
			res[0] = unicode.ToUpper(runes[0])
			for i := 1; i < len(runes); i++ {
				res[i] = unicode.ToLower(runes[i])
			}
			return string(res)
		}
	}
	return word
}

func cycleCase(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}

	isAllLower, isAllUpper, hasLetters := true, true, false
	for _, r := range runes {
		if unicode.IsLetter(r) {
			hasLetters = true
			if !unicode.IsLower(r) {
				isAllLower = false
			}
			if !unicode.IsUpper(r) {
				isAllUpper = false
			}
		}
	}
	if !hasLetters {
		return text
	}
	if isAllLower {
		return strings.ToUpper(text)
	}
	if isAllUpper {
		res := []rune(strings.ToLower(text))
		capitalizeNext := true
		for i, r := range res {
			if unicode.IsLetter(r) {
				if capitalizeNext {
					res[i] = unicode.ToUpper(r)
					capitalizeNext = false
				}
			} else if unicode.IsSpace(r) || strings.ContainsRune("-_\"'([{", r) {
				capitalizeNext = true
			}
		}
		return string(res)
	}
	return strings.ToLower(text)
}

func convertSelectedLayout(text string) (string, bool) {
	ruCount, enCount := 0, 0
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			ruCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			enCount++
		}
	}
	toEn := ruCount >= enCount
	var res strings.Builder
	for _, r := range text {
		if toEn {
			if mapped, ok := fullRuToEn[r]; ok {
				res.WriteRune(mapped)
			} else if r == '.' {
				res.WriteRune('/')
			} else {
				res.WriteRune(r)
			}
		} else {
			if mapped, ok := fullEnToRu[r]; ok {
				res.WriteRune(mapped)
			} else {
				res.WriteRune(r)
			}
		}
	}
	return res.String(), toEn
}
