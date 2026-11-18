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
	LogPriorPos float64            `json:"log_prior_pos"`
	LogPriorNeg float64            `json:"log_prior_neg"`
	LogProbPos  map[string]float64 `json:"log_prob_pos"`
	LogProbNeg  map[string]float64 `json:"log_prob_neg"`
	UnseenPos   float64            `json:"unseen_pos"`
	UnseenNeg   float64            `json:"unseen_neg"`
}

type AIModel struct {
	Version      int             `json:"version"`
	Russian      *LanguageModel  `json:"russian"`
	English      *LanguageModel  `json:"english"`
	KnownWords   map[string]bool `json:"known_words"`
	ValidGramsEn map[string]bool `json:"valid_grams_en"`
	ValidGramsRu map[string]bool `json:"valid_grams_ru"`
}

var currentModel *AIModel

var commonWords = map[string]bool{
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
	"и": true, "в": true, "не": true, "на": true, "я": true, "быть": true, "он": true,
	"с": true, "что": true, "а": true, "по": true, "это": true, "она": true, "этот": true,
	"к": true, "но": true, "они": true, "мы": true, "как": true, "из": true, "у": true,
	"тот": true, "за": true, "свой": true, "её": true, "так": true, "же": true, "от": true,
	"для": true, "все": true, "или": true, "при": true, "бы": true, "его": true, "до": true,
	"вас": true, "ваш": true, "вот": true, "уже": true, "вы": true, "нет": true,
	"да": true, "если": true, "чтобы": true, "только": true, "себя": true, "мне": true,
}

var devOpsTerms = []string{
	"users", "limit", "kind", "promt", "grep", "awk", "sed", "chmod", "chown", "root", "sudo",
	"localhost", "userspace", "backend", "frontend", "config", "yaml", "json", "bash", "shell", "fish",
	"kubernetes", "kubectl", "helm", "docker", "ansible", "terraform", "traefik", "nginx",
	"opensearch", "elasticsearch", "kafka", "vector", "victorialogs", "galera", "percona",
	"linux", "ubuntu", "centos", "opensuse", "tumbleweed", "wsl", "hyperv", "vmware", "virtualbox",
	"mikrotik", "routeros", "winbox", "wireguard", "ssh", "proxy", "network", "node", "cluster", "deploy",
	"git", "commit", "vscode", "zed", "golang", "rust", "python", "script",
}

var ruToEn = map[rune]rune{'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']', 'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'', 'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`'}
var enToRu = map[rune]rune{'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш', 'o': 'щ', 'p': 'з', '[': 'х', ']': 'ъ', 'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л', 'l': 'д', ';': 'ж', '\'': 'э', 'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь', ',': 'б', '.': 'ю', '`': 'ё'}

// ⚡ ВАРИАНТ В: Топография клавиатуры (Проекция физического расположения клавиш)
type KeyCoord struct {
	X, Y float64
}

// Координатная сетка смещенных рядов стандартной клавиатуры
var qwertyMap = map[rune]KeyCoord{
	'q': {0, 0}, 'w': {1, 0}, 'e': {2, 0}, 'r': {3, 0}, 't': {4, 0}, 'y': {5, 0}, 'u': {6, 0}, 'i': {7, 0}, 'o': {8, 0}, 'p': {9, 0}, '[': {10, 0}, ']': {11, 0},
	'a': {0.5, 1}, 's': {1.5, 1}, 'd': {2.5, 1}, 'f': {3.5, 1}, 'g': {4.5, 1}, 'h': {5.5, 1}, 'j': {6.5, 1}, 'k': {7.5, 1}, 'l': {8.5, 1}, ';': {9.5, 1}, '\'': {10.5, 1},
	'z': {1, 2}, 'x': {2, 2}, 'c': {3, 2}, 'v': {4, 2}, 'b': {5, 2}, 'n': {6, 2}, 'm': {7, 2}, ',': {8, 2}, '.': {9, 2}, '/': {10, 2},
}

// Высчитывает среднюю дистанцию пробега пальцев при наборе слова
func calcAverageJump(physicalWordEn string) float64 {
	w := strings.ToLower(physicalWordEn)
	if len(w) < 2 {
		return 3.0 // Дефолтное нормальное расстояние
	}

	var totalDist float64
	var jumps float64
	var lastCoord KeyCoord
	var hasLast bool

	for _, r := range w {
		if coord, ok := qwertyMap[r]; ok {
			if hasLast {
				// Теорема Пифагора для поиска дистанции между клавишами
				dx := coord.X - lastCoord.X
				dy := coord.Y - lastCoord.Y
				dist := math.Sqrt(dx*dx + dy*dy)
				totalDist += dist
				jumps++
			}
			lastCoord = coord
			hasLast = true
		}
	}

	if jumps == 0 {
		return 3.0
	}
	return totalDist / jumps
}

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
	return words
}

func sanitizeDictionary(words []string, vowels, consonants string) []string {
	var clean []string
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		if len([]rune(w)) >= 3 && !strings.ContainsAny(w, vowels) {
			continue
		}
		cur := 0
		for _, r := range w {
			if strings.ContainsRune(consonants, r) {
				cur++
			} else {
				cur = 0
			}
			if cur > 4 {
				break
			}
		}
		if cur > 4 {
			continue
		}
		clean = append(clean, w)
	}
	return clean
}

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

// ⚡ Динамический штраф Лапласа из настроек (penalty)
func trainNaiveBayes(positives, negatives []string, penalty float64) *LanguageModel {
	posCount := float64(len(positives))
	negCount := float64(len(negatives))
	totalCount := posCount + negCount

	lm := &LanguageModel{
		LogPriorPos: math.Log(posCount / totalCount),
		LogPriorNeg: math.Log(negCount / totalCount),
		LogProbPos:  make(map[string]float64),
		LogProbNeg:  make(map[string]float64),
	}

	posGramsCount := make(map[string]float64)
	negGramsCount := make(map[string]float64)
	vocab := make(map[string]bool)

	var totalPosGrams, totalNegGrams float64

	for _, w := range positives {
		for _, ng := range getNGrams(w) {
			posGramsCount[ng]++
			vocab[ng] = true
			totalPosGrams++
		}
	}

	for _, w := range negatives {
		for _, ng := range getNGrams(w) {
			negGramsCount[ng]++
			vocab[ng] = true
			totalNegGrams++
		}
	}

	vocabSize := float64(len(vocab))

	for ng := range vocab {
		lm.LogProbPos[ng] = math.Log((posGramsCount[ng] + penalty) / (totalPosGrams + vocabSize*penalty))
		lm.LogProbNeg[ng] = math.Log((negGramsCount[ng] + penalty) / (totalNegGrams + vocabSize*penalty))
	}

	lm.UnseenPos = math.Log(penalty / (totalPosGrams + vocabSize*penalty))
	lm.UnseenNeg = math.Log(penalty / (totalNegGrams + vocabSize*penalty))

	return lm
}

func TrainModel(ruPath, enPath string) error {
	// Считываем динамические параметры из Web UI
	settingsMutex.RLock()
	limit := settings.TrainingLimit
	if limit <= 0 {
		limit = 60000
	}
	penalty := float64(settings.LaplacePenalty)
	if penalty <= 0.001 { // Защита от нулевого сглаживания
		penalty = 1.0
	}
	customVocabStr := settings.CustomVocab
	settingsMutex.RUnlock()

	fmt.Println("[ИИ] Загрузка и очистка словарей от мусора...")

	rawRu := loadWords(ruPath)
	rawEn := loadWords(enPath)

	ruWords := sanitizeDictionary(rawRu, "аеёиоуыэюя", "бвгджзйклмнпрстфхцчшщ")
	enWords := sanitizeDictionary(rawEn, "aeiouy", "bcdfghjklmnpqrstvwxz")

	// Инъекция стандартных терминов
	for _, term := range devOpsTerms {
		enWords = append(enWords, term)
	}

	// ⚡ Инъекция пользовательских терминов из Web UI
	for _, term := range strings.Split(customVocabStr, "\n") {
		term = strings.TrimSpace(term)
		if term != "" {
			enWords = append(enWords, term)
		}
	}

	fmt.Printf("[ИИ] Очищено: RU=%d (было %d), EN=%d (было %d)\n", len(ruWords), len(rawRu), len(enWords), len(rawEn))

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(ruWords), func(i, j int) { ruWords[i], ruWords[j] = ruWords[j], ruWords[i] })
	rnd.Shuffle(len(enWords), func(i, j int) { enWords[i], enWords[j] = enWords[j], enWords[i] })

	ruSample := ruWords
	if len(ruSample) > limit {
		ruSample = ruSample[:limit]
	}
	enSample := enWords
	if len(enSample) > limit {
		enSample = enSample[:limit]
	}

	fakeRu := make([]string, 0, len(enSample))
	for _, w := range enSample {
		fakeRu = append(fakeRu, convertEnToRu(w))
	}
	fakeEn := make([]string, 0, len(ruSample))
	for _, w := range ruSample {
		fakeEn = append(fakeEn, convertRuToEn(w))
	}

	model := &AIModel{
		Version:      3,
		KnownWords:   make(map[string]bool),
		ValidGramsEn: make(map[string]bool),
		ValidGramsRu: make(map[string]bool),
	}

	for _, w := range ruWords {
		model.KnownWords[w] = true
		marked := "^" + w + "$"
		chars := []rune(marked)
		for i := 0; i <= len(chars)-3; i++ {
			model.ValidGramsRu[string(chars[i:i+3])] = true
		}
	}
	for _, w := range enWords {
		model.KnownWords[w] = true
		marked := "^" + w + "$"
		chars := []rune(marked)
		for i := 0; i <= len(chars)-3; i++ {
			model.ValidGramsEn[string(chars[i:i+3])] = true
		}
	}

	fmt.Printf("[ИИ] Сборка русской модели (Лимит: %d, Лаплас: %.1f)...\n", limit, penalty)
	ruNeg := make([]string, 0, len(fakeRu)+len(enSample)/4)
	ruNeg = append(ruNeg, fakeRu...)
	for i := 0; i < len(enSample) && i < 15000; i++ {
		ruNeg = append(ruNeg, enSample[i])
	}
	model.Russian = trainNaiveBayes(ruSample, ruNeg, penalty)

	fmt.Println("[ИИ] Сборка английской модели...")
	enNeg := make([]string, 0, len(fakeEn)+len(ruSample)/4)
	enNeg = append(enNeg, fakeEn...)
	for i := 0; i < len(ruSample) && i < 15000; i++ {
		enNeg = append(enNeg, ruSample[i])
	}
	model.English = trainNaiveBayes(enSample, enNeg, penalty)

	fmt.Println("[ИИ] Сохранение model.json (Формат V3)...")
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

	if m.Version >= 3 && m.Russian != nil && m.English != nil {
		currentModel = &m
		logDebug("Модель v3 загружена")
		return
	}

	logDebug("Модель старого формата — требуется переобучение (флаг -learn)")
}

func IsImpossible(word string) bool {
	w := strings.ToLower(word)
	if len(w) < 3 {
		return false
	}
	hasVowels := strings.ContainsAny(w, "аеёиоуыэюяaeiouy")
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

func isCyrillic(word string) bool {
	for _, r := range word {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func IsHardTypo(word string, isRu bool, isFinished bool) bool {
	if len(word) < 2 {
		return false
	}
	if IsImpossible(word) {
		return true
	}
	if currentModel == nil || currentModel.ValidGramsEn == nil || currentModel.ValidGramsRu == nil {
		return false
	}
	validMap := currentModel.ValidGramsEn
	if isRu {
		validMap = currentModel.ValidGramsRu
	}

	marked := "^" + word
	if isFinished {
		marked += "$"
	}
	chars := []rune(marked)

	for i := 0; i <= len(chars)-3; i++ {
		if !validMap[string(chars[i:i+3])] {
			return true
		}
	}
	return false
}

// ⚡ ТОЧНЫЙ РАСЧЕТ: Суммирование логарифмических вероятностей
func (lm *LanguageModel) Score(word string) float64 {
	if lm == nil || lm.LogProbPos == nil {
		return 0
	}
	scorePos := lm.LogPriorPos
	scoreNeg := lm.LogPriorNeg

	for _, ng := range getNGrams(word) {
		if val, ok := lm.LogProbPos[ng]; ok {
			scorePos += val
		} else {
			scorePos += lm.UnseenPos
		}

		if val, ok := lm.LogProbNeg[ng]; ok {
			scoreNeg += val
		} else {
			scoreNeg += lm.UnseenNeg
		}
	}

	diff := scoreNeg - scorePos
	// Защита от математического переполнения
	if diff > 500 {
		return 0.0
	}
	if diff < -500 {
		return 1.0
	}
	return 1.0 / (1.0 + math.Exp(diff))
}

func scoreInLang(word, lang string) float64 {
	if currentModel == nil {
		return 0
	}
	if lang == "ru" {
		return currentModel.Russian.Score(word)
	}
	return currentModel.English.Score(word)
}

func ShouldConvertWord(wordCurrent, wordOther, currentLang string, threshold float64) bool {
	wc := strings.ToLower(wordCurrent)
	wo := strings.ToLower(wordOther)

	if len(wc) < 2 {
		return false
	}
	if commonWords[wc] {
		return false
	}

	isRu := (currentLang == "ru")

	if currentModel == nil {
		if IsImpossible(wc) && !IsImpossible(wo) {
			return true
		}
		return false
	}

	if currentModel.KnownWords[wc] {
		return false
	}

	if currentModel.KnownWords[wo] {
		return true
	}

	if IsHardTypo(wc, isRu, true) && !IsHardTypo(wo, !isRu, true) {
		return true
	}

	var scoreCurrent, scoreOther float64
	var physicalPathEn string

	if isRu {
		scoreCurrent = scoreInLang(wc, "ru")
		scoreOther = scoreInLang(wo, "en")
		physicalPathEn = wo // Если текущий русский, физические клавиши отражены в английской проекции
	} else {
		scoreCurrent = scoreInLang(wc, "en")
		scoreOther = scoreInLang(wo, "ru")
		physicalPathEn = wc
	}

	// ⚡ Топографический модификатор
	avgJump := calcAverageJump(physicalPathEn)
	jumpModifier := 0.0

	// Если пальцы бьют в одну кучу (дистанция < 2.0), это с высокой вероятностью опечатка
	if avgJump < 2.0 {
		jumpModifier = 0.10 // Смягчаем порог переключения (повышаем шанс замены)
	} else if avgJump > 4.5 {
		jumpModifier = -0.05 // Слишком широкие прыжки — скорее всего нормальное слово
	}

	margin := 0.05 + (threshold-0.5)*0.5 - jumpModifier

	// Защитные рамки маржи
	if margin < 0.02 {
		margin = 0.02
	}
	if margin > 0.40 {
		margin = 0.40
	}

	return (scoreOther-scoreCurrent) > margin && scoreOther > 0.35
}
