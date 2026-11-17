package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type LanguageModel struct {
	Weights map[string]float64 `json:"weights"`
	Bias    float64            `json:"bias"`
}

type AIModel struct {
	Version    int             `json:"version"`
	Russian    *LanguageModel  `json:"russian"`
	English    *LanguageModel  `json:"english"`
	KnownWords map[string]bool `json:"known_words"`
}

var currentModel *AIModel

// Только очень частые слова — жёсткий стоп-лист против ложных срабатываний
var commonWords = map[string]bool{
	// English
	"the": true, "be": true, "to": true, "of": true, "and": true, "a": true, "in": true,
	"that": true, "have": true, "i": true, "it": true, "for": true, "not": true, "on": true,
	"with": true, "he": true, "as": true, "you": true, "do": true, "at": true, "this": true,
	"but": true, "his": true, "by": true, "from": true, "they": true, "we": true, "say": true,
	"her": true, "she": true, "or": true, "an": true, "will": true, "my": true, "one": true,
	"all": true, "would": true, "there": true, "their": true, "what": true, "so": true,
	"up": true, "out": true, "if": true, "about": true, "who": true, "get": true, "which": true,
	"go": true, "me": true, "when": true, "make": true, "can": true, "like": true, "time": true,
	"no": true, "just": true, "him": true, "know": true, "take": true, "people": true, "into": true,
	"year": true, "your": true, "good": true, "some": true, "could": true, "them": true,
	"see": true, "other": true, "than": true, "then": true, "now": true, "look": true,
	"only": true, "come": true, "its": true, "over": true, "think": true, "also": true,
	// Russian
	"и": true, "в": true, "не": true, "на": true, "я": true, "быть": true, "он": true,
	"с": true, "что": true, "а": true, "по": true, "это": true, "она": true, "этот": true,
	"к": true, "но": true, "они": true, "мы": true, "как": true, "из": true, "у": true,
	"тот": true, "за": true, "свой": true, "её": true, "так": true, "же": true, "от": true,
	"для": true, "все": true, "или": true, "при": true, "бы": true, "его": true, "до": true,
	"вас": true, "ваш": true, "вот": true, "уже": true, "вы": true, "нет": true,
	"да": true, "если": true, "чтобы": true, "только": true, "себя": true, "мне": true,
}

var ruToEn = map[rune]rune{'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']', 'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'', 'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`'}
var enToRu = map[rune]rune{'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш', 'o': 'щ', 'p': 'з', '[': 'х', ']': 'ъ', 'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л', 'l': 'д', ';': 'ж', '\'': 'э', 'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь', ',': 'б', '.': 'ю', '`': 'ё'}

func convertRuToEn(word string) string {
	var res strings.Builder
	for _, r := range []rune(word) {
		lowerR := unicode.ToLower(r)
		if enChar, ok := ruToEn[lowerR]; ok {
			if unicode.IsUpper(r) {
				res.WriteRune(unicode.ToUpper(enChar))
			} else {
				res.WriteRune(enChar)
			}
		} else {
			res.WriteRune(r)
		}
	}
	return res.String()
}

func convertEnToRu(word string) string {
	var res strings.Builder
	for _, r := range []rune(word) {
		lowerR := unicode.ToLower(r)
		if ruChar, ok := enToRu[lowerR]; ok {
			if unicode.IsUpper(r) {
				res.WriteRune(unicode.ToUpper(ruChar))
			} else {
				res.WriteRune(ruChar)
			}
		} else {
			res.WriteRune(r)
		}
	}
	return res.String()
}

func loadWords(path string) []string {
	var words []string
	file, err := os.Open(path)
	if err != nil {
		return words
	}
	defer file.Close()

	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		w := scanner.Text()
		w = strings.TrimPrefix(w, "\xef\xbb\xbf")
		w = strings.TrimSpace(w)
		w = strings.ToLower(w)
		if w != "" && !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("[-] Ошибка чтения %s: %v\n", path, err)
	}
	return words
}

// n-граммы с маркерами начала/конца слова
func getNGrams(word string) []string {
	marked := "^" + word + "$"
	chars := []rune(marked)
	var ngrams []string
	for n := 2; n <= 4; n++ {
		for i := 0; i <= len(chars)-n; i++ {
			ngrams = append(ngrams, string(chars[i:i+n]))
		}
	}
	return ngrams
}

// Score возвращает P(слово валидно в языке модели)
func (lm *LanguageModel) Score(word string) float64 {
	if lm == nil || lm.Weights == nil {
		return 0
	}
	logit := lm.Bias
	for _, ng := range getNGrams(word) {
		logit += lm.Weights[ng]
	}
	return 1.0 / (1.0 + math.Exp(-logit))
}

func trainLanguageModel(positives, negatives []string, epochs int, initialLr float64) *LanguageModel {
	lm := &LanguageModel{Weights: make(map[string]float64)}

	type sample struct {
		word  string
		label float64
	}

	dataset := make([]sample, 0, len(positives)+len(negatives))
	for _, w := range positives {
		dataset = append(dataset, sample{w, 1})
	}
	for _, w := range negatives {
		dataset = append(dataset, sample{w, 0})
	}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	for e := 0; e < epochs; e++ {
		lr := initialLr * (1.0 - 0.5*float64(e)/float64(epochs))
		rnd.Shuffle(len(dataset), func(i, j int) { dataset[i], dataset[j] = dataset[j], dataset[i] })

		for _, s := range dataset {
			ngrams := getNGrams(s.word)
			logit := lm.Bias
			for _, ng := range ngrams {
				logit += lm.Weights[ng]
			}
			pred := 1.0 / (1.0 + math.Exp(-logit))
			err := s.label - pred
			for _, ng := range ngrams {
				lm.Weights[ng] += lr * err
			}
			lm.Bias += lr * err
		}
		fmt.Printf("[ИИ] Эпоха %d/%d (lr=%.4f)\n", e+1, epochs, lr)
	}
	return lm
}

func TrainModel(ruPath, enPath string) error {
	fmt.Println("[ИИ] Обучение двух моделей...")
	ruWords := loadWords(ruPath)
	enWords := loadWords(enPath)
	fmt.Printf("[ИИ] Загружено: RU=%d, EN=%d\n", len(ruWords), len(enWords))

	// Ограничим выборку для скорости
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(ruWords), func(i, j int) { ruWords[i], ruWords[j] = ruWords[j], ruWords[i] })
	rnd.Shuffle(len(enWords), func(i, j int) { enWords[i], enWords[j] = enWords[j], enWords[i] })

	limit := 60000
	ruSample := ruWords
	if len(ruSample) > limit {
		ruSample = ruSample[:limit]
	}
	enSample := enWords
	if len(enSample) > limit {
		enSample = enSample[:limit]
	}

	// Генерируем «фейковые» слова — транслит в противоположной раскладке
	fakeRu := make([]string, 0, len(enSample)) // EN → RU-подобные
	for _, w := range enSample {
		fakeRu = append(fakeRu, convertEnToRu(w))
	}
	fakeEn := make([]string, 0, len(ruSample)) // RU → EN-подобные
	for _, w := range ruSample {
		fakeEn = append(fakeEn, convertRuToEn(w))
	}

	model := &AIModel{
		Version:    2,
		KnownWords: make(map[string]bool),
	}

	for _, w := range ruWords {
		model.KnownWords[w] = true
	}
	for _, w := range enWords {
		model.KnownWords[w] = true
	}

	// RU-модель: учим отличать настоящий русский от фейкового (транслит EN) и латиницы
	fmt.Println("[ИИ] Обучение русской модели...")
	ruNeg := make([]string, 0, len(fakeRu)+len(enSample)/4)
	ruNeg = append(ruNeg, fakeRu...)
	for i := 0; i < len(enSample) && i < 15000; i++ {
		ruNeg = append(ruNeg, enSample[i]) // латиница как явный negative
	}
	model.Russian = trainLanguageModel(ruSample, ruNeg, 12, 0.15)

	// EN-модель: настоящий английский vs фейковый (транслит RU) и кириллица
	fmt.Println("[ИИ] Обучение английской модели...")
	enNeg := make([]string, 0, len(fakeEn)+len(ruSample)/4)
	enNeg = append(enNeg, fakeEn...)
	for i := 0; i < len(ruSample) && i < 15000; i++ {
		enNeg = append(enNeg, ruSample[i])
	}
	model.English = trainLanguageModel(enSample, enNeg, 12, 0.15)

	fmt.Println("[ИИ] Сохранение model.json...")
	data, _ := json.MarshalIndent(model, "", "  ")
	if err := os.WriteFile("model.json", data, 0644); err != nil {
		return err
	}
	fmt.Println("[ИИ] Готово.")
	return nil
}

func loadModel() {
	path := filepath.Join(getAppDir(), "model.json")
	data, err := os.ReadFile(path)
	if err != nil {
		logDebug("model.json не найден: %s", path)
		return
	}

	var m AIModel
	if err := json.Unmarshal(data, &m); err != nil {
		logDebug("Ошибка парсинга модели: %v", err)
		return
	}

	// Проверка новой структуры
	if m.Version >= 2 && m.Russian != nil && m.English != nil {
		currentModel = &m
		logDebug("Модель v2 загружена (RU=%d весов, EN=%d весов)",
			len(m.Russian.Weights), len(m.English.Weights))
		return
	}

	logDebug("Модель старого формата — требуется переобучение (флаг -learn)")
}

// IsImpossible — эвристика для «явно не слово в текущей раскладке»
func IsImpossible(word string) bool {
	w := strings.ToLower(word)
	if len(w) < 3 {
		return false
	}
	hasVowels := strings.ContainsAny(w, "аеёиоуыэюяaeiouy")
	// Согласных подряд много — почти всегда транслит
	consonants := "бвгджзйклмнпрстфхцчшщbcdfghjklmnpqrstvwxz"
	maxCons := 0
	cur := 0
	for _, r := range w {
		if strings.ContainsRune(consonants, r) {
			cur++
			if cur > maxCons {
				maxCons = cur
			}
		} else {
			cur = 0
		}
	}
	return (!hasVowels && len(w) >= 3) || maxCons >= 5
}

// ScoreCurrent — вероятность что слово валидно в языке lang
func scoreInLang(word, lang string) float64 {
	if currentModel == nil {
		return 0
	}
	if lang == "ru" {
		return currentModel.Russian.Score(word)
	}
	return currentModel.English.Score(word)
}

// ShouldConvertWord — главная точка принятия решения
func ShouldConvertWord(wordCurrent, wordOther, currentLang string, threshold float64) bool {
	wc := strings.ToLower(wordCurrent)
	wo := strings.ToLower(wordOther)

	if len(wc) < 2 {
		return false
	}
	if commonWords[wc] {
		return false
	}

	// Без модели — простая эвристика
	if currentModel == nil {
		if IsImpossible(wc) && !IsImpossible(wo) {
			return true
		}
		return false
	}

	// Текущее слово — реальное слово в любом из языков → не трогаем
	if currentModel.KnownWords[wc] {
		return false
	}

	// Другое слово — реальное слово → почти всегда конвертируем
	if currentModel.KnownWords[wo] {
		return true
	}

	// Явно невозможное текущее → конвертируем
	if IsImpossible(wc) && !IsImpossible(wo) {
		return true
	}

	// Сравниваем уверенность двух моделей
	var scoreCurrent, scoreOther float64
	if currentLang == "ru" {
		scoreCurrent = scoreInLang(wc, "ru") // насколько текущее похоже на русское
		scoreOther = scoreInLang(wo, "en")   // насколько другое похоже на английское
	} else {
		scoreCurrent = scoreInLang(wc, "en")
		scoreOther = scoreInLang(wo, "ru")
	}

	// margin растёт с порогом: 0.5→агрессивно, 0.9→консервативно
	margin := 0.05 + (threshold-0.5)*0.5
	if margin < 0.05 {
		margin = 0.05
	}
	if margin > 0.35 {
		margin = 0.35
	}

	return (scoreOther-scoreCurrent) > margin && scoreOther > 0.35
}
