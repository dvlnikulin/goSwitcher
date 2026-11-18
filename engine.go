package main

import (
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

type KeyEventCore struct {
	CharEn string
	CharRu string
	Shift  bool
	Caps   bool
}

type CorrectionData struct {
	OriginalFull  string
	OriginalClean string
	PhysicalEn    string
	Corrected     string
	AppendChar    string
	SwitchedLang  bool
	Timestamp     time.Time
}

var (
	currentWord             []KeyEventCore
	isReplacing             uint32
	lastWord                []KeyEventCore
	typedSpaceAfterLastWord bool
	lastRenderedWord        string
	lastRenderedSuffix      string
	lastAutoCorrection      *CorrectionData
	backspaceCount          int
	lastShiftTime           time.Time
	lastFinalLang           string = "en"
)

func buildWordStrings(events []KeyEventCore) (string, string, bool, bool, bool, bool) {
	var enStr, ruStr strings.Builder
	var hasUpper, hasLower, hasDigit, hasCapsLock bool
	for i, ev := range events {
		if ev.Caps {
			hasCapsLock = true
		}
		if len(ev.CharEn) == 1 && ev.CharEn[0] >= '0' && ev.CharEn[0] <= '9' {
			hasDigit = true
		}
		if len(ev.CharEn) > 0 {
			r := []rune(ev.CharEn)[0]
			if unicode.IsUpper(r) {
				if i > 0 {
					hasUpper = true
				}
			}
			if unicode.IsLower(r) {
				if i > 0 {
					hasLower = true
				}
			}
		}
		enStr.WriteString(ev.CharEn)
		ruStr.WriteString(ev.CharRu)
	}
	return enStr.String(), ruStr.String(), hasUpper, hasLower, hasDigit, hasCapsLock
}

func replaceText(deletions int, replacement string, appendChar string, switchLang bool) {
	lastRenderedWord = replacement
	lastRenderedSuffix = appendChar

	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(30 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	time.Sleep(15 * time.Millisecond)
	totalDel := deletions
	if appendChar != "" {
		totalDel += len([]rune(appendChar))
	}
	for i := 0; i < totalDel; i++ {
		osSimulateBackspace()
		time.Sleep(3 * time.Millisecond)
	}

	time.Sleep(10 * time.Millisecond)
	osSendString(replacement + appendChar)
	time.Sleep(25 * time.Millisecond)

	if switchLang {
		osSwitchLayout()
	}
}

func tryTransformSelection(mode string) bool {
	atomic.StoreUint32(&isReplacing, 1)
	defer func() {
		time.Sleep(40 * time.Millisecond)
		atomic.StoreUint32(&isReplacing, 0)
	}()

	backup, hadBackup := osGetClipboard()
	osSetClipboard("")

	osSendCtrlKey("c")
	time.Sleep(75 * time.Millisecond)

	selected, ok := osGetClipboard()
	if !ok || selected == "" {
		if hadBackup {
			osSetClipboard(backup)
		}
		return false
	}

	var transformed string
	var switchedToEn bool
	if mode == "layout" {
		transformed, switchedToEn = convertSelectedLayout(selected)
	} else {
		transformed = cycleCase(selected)
	}

	if transformed == selected {
		if hadBackup {
			osSetClipboard(backup)
		}
		return true
	}

	osSetClipboard(transformed)
	time.Sleep(20 * time.Millisecond)
	osSendCtrlKey("v")
	time.Sleep(75 * time.Millisecond)

	if mode == "layout" {
		isRu := osIsRussianLayout()
		if (isRu && switchedToEn) || (!isRu && !switchedToEn) {
			osSwitchLayout()
		}
	}

	if hadBackup {
		osSetClipboard(backup)
	} else {
		osSetClipboard("")
	}
	EngineClearContext()
	return true
}

func EngineClearContext() {
	currentWord = currentWord[:0]
	lastWord = lastWord[:0]
	lastRenderedWord = ""
}

func EngineAddChar(charEn, charRu string, isShift, isCaps bool) {
	if charEn != "" && isBoundaryEn(charEn) {
		processWord(charEn, false)
	} else if charEn != "" {
		currentWord = append(currentWord, KeyEventCore{CharEn: charEn, CharRu: charRu, Shift: isShift, Caps: isCaps})

		settingsMutex.RLock()
		liveTypoEnabled := settings.LiveTypoEnabled
		liveTypoChar := settings.LiveTypoChar
		settingsMutex.RUnlock()

		if liveTypoChar < 3 {
			liveTypoChar = 4
		}
		if liveTypoEnabled && len(currentWord) >= liveTypoChar {
			processLiveTypo()
		}
	} else {
		currentWord = currentWord[:0]
	}
}

func EngineHandleBackspace() {
	if lastAutoCorrection != nil && time.Since(lastAutoCorrection.Timestamp) < 15*time.Second {
		backspaceCount++
		targetLen := len([]rune(lastAutoCorrection.Corrected)) + len([]rune(lastAutoCorrection.AppendChar))
		if backspaceCount >= targetLen {
			learnException(lastAutoCorrection.OriginalClean)
			if lastAutoCorrection.SwitchedLang {
				revertHeatmapError(lastAutoCorrection.PhysicalEn)
			}
			incrementUndoCount(len([]rune(lastAutoCorrection.OriginalClean)), lastAutoCorrection.SwitchedLang)
			lastAutoCorrection = nil
			backspaceCount = 0
		}
	} else {
		backspaceCount = 0
	}

	if len(currentWord) > 0 {
		currentWord = currentWord[:len(currentWord)-1]
	} else {
		lastWord = lastWord[:0]
		lastRenderedWord = ""
	}
}

func EngineHandleDoubleShift() {
	now := time.Now()
	if now.Sub(lastShiftTime) < 400*time.Millisecond {
		if lastAutoCorrection != nil && time.Since(lastAutoCorrection.Timestamp) < 15*time.Second {
			deletions := len([]rune(lastAutoCorrection.Corrected))
			go replaceText(deletions, lastAutoCorrection.OriginalFull, lastAutoCorrection.AppendChar, lastAutoCorrection.SwitchedLang)
			learnException(lastAutoCorrection.OriginalClean)
			if lastAutoCorrection.SwitchedLang {
				revertHeatmapError(lastAutoCorrection.PhysicalEn)
			}
			incrementUndoCount(len([]rune(lastAutoCorrection.OriginalClean)), lastAutoCorrection.SwitchedLang)
			lastAutoCorrection = nil
		}
		lastShiftTime = time.Time{}
	} else {
		lastShiftTime = now
	}
}

func EngineHandleSpace() { processWord(" ", false) }
func EngineHandleEnter() { processWord("\n", false) }
func EngineHandleTab()   { processWord("\t", false) }
func EngineHandleManualSwitch() {
	if !tryTransformSelection("layout") {
		processWord("", true)
	}
}

func EngineHandleCaseToggle() {
	if tryTransformSelection("case") {
		return
	}
	isRu := osIsRussianLayout()
	if len(currentWord) > 0 {
		wordEn, wordRu, _, _, _, _ := buildWordStrings(currentWord)
		target := wordEn
		if isRu {
			target = wordRu
		}
		deletions := len([]rune(target))
		cycled := cycleCase(target)
		lastWord = make([]KeyEventCore, len(currentWord))
		copy(lastWord, currentWord)
		typedSpaceAfterLastWord = false
		currentWord = currentWord[:0]
		replaceText(deletions, cycled, "", false)
		return
	}
	if lastRenderedWord != "" {
		deletions := len([]rune(lastRenderedWord))
		cycled := cycleCase(lastRenderedWord)
		replaceText(deletions, cycled, lastRenderedSuffix, false)
	}
}

func learnException(word string) {
	word = strings.TrimSpace(strings.ToLower(word))
	if len(word) < 2 {
		return
	}
	if currentModel != nil && currentModel.KnownWords != nil {
		currentModel.KnownWords[word] = true
	}
	settingsMutex.RLock()
	existing := settings.Exceptions
	newSettings := settings
	settingsMutex.RUnlock()

	for _, e := range strings.Split(existing, "\n") {
		if strings.TrimSpace(e) == word {
			return
		}
	}
	if newSettings.Exceptions != "" && !strings.HasSuffix(newSettings.Exceptions, "\n") {
		newSettings.Exceptions += "\n"
	}
	newSettings.Exceptions += word
	go saveSettings(newSettings)
}

func learnForceConvert(word string) {
	word = strings.TrimSpace(strings.ToLower(word))
	if len(word) < 2 {
		return
	}
	settingsMutex.RLock()
	existing := settings.ForceConvert
	newSettings := settings
	settingsMutex.RUnlock()

	for _, e := range strings.Split(existing, "\n") {
		if strings.TrimSpace(e) == word {
			return
		}
	}
	if newSettings.ForceConvert != "" && !strings.HasSuffix(newSettings.ForceConvert, "\n") {
		newSettings.ForceConvert += "\n"
	}
	newSettings.ForceConvert += word
	go saveSettings(newSettings)
}

func processLiveTypo() {
	settingsMutex.RLock()
	aiEnabled := settings.EnableAI
	exceptions := strings.ToLower(settings.Exceptions)
	blockFlags := settings.BlockFlags
	blockPaths := settings.BlockPaths
	settingsMutex.RUnlock()

	if !aiEnabled || len(currentWord) < 3 {
		return
	}
	wordEn, wordRu, hasUpper, hasLower, hasDigit, hasCapsLock := buildWordStrings(currentWord)
	wLowerEn, wLowerRu := strings.ToLower(wordEn), strings.ToLower(wordRu)
	cEn, cRu := cleanWord(wordEn, "en"), cleanWord(wordRu, "ru")

	for _, excLine := range strings.Split(exceptions, "\n") {
		excLine = strings.TrimSpace(excLine)
		if excLine == "" {
			continue
		}
		if wLowerEn == excLine || wLowerRu == excLine || cEn == excLine || cRu == excLine {
			return
		}
	}

	isRu := osIsRussianLayout()
	deletions := len(currentWord)

	if isITPattern(wordEn) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			EngineClearContext()
		}
		return
	}
	if hasDigit || (hasUpper && hasLower && !hasCapsLock) {
		return
	}

	isFlag := strings.HasPrefix(wordEn, "-")
	isPath := strings.HasPrefix(wordEn, "\\") || strings.HasPrefix(wordEn, "/")

	if (isFlag && blockFlags) || (isPath && blockPaths) {
		if isRu {
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, "", true)
			EngineClearContext()
		}
		return
	}

	if isRu {
		if IsHardTypo(cRu, true, false) && !IsHardTypo(cEn, false, false) {
			finalEn := wordEn
			if hasCapsLock {
				finalEn = fixInvertedCaps(finalEn)
			}
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordRu,
				OriginalClean: cRu,
				PhysicalEn:    wLowerEn,
				Corrected:     finalEn,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "en"
			EngineClearContext()
			go replaceText(deletions, finalEn, "", true)
		}
	} else {
		if IsHardTypo(cEn, false, false) && !IsHardTypo(cRu, true, false) {
			finalRu := wordRu
			if hasCapsLock {
				finalRu = fixInvertedCaps(finalRu)
			}
			lastAutoCorrection = &CorrectionData{
				OriginalFull:  wordEn,
				OriginalClean: cEn,
				PhysicalEn:    wLowerEn,
				Corrected:     finalRu,
				SwitchedLang:  true,
				Timestamp:     time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "ru"
			EngineClearContext()
			go replaceText(deletions, finalRu, "", true)
		}
	}
}

func processWord(appendChar string, isManual bool) {
	settingsMutex.RLock()
	aiEnabled := settings.EnableAI
	threshold := float64(settings.Threshold) / 100.0
	exceptions := strings.ToLower(settings.Exceptions)
	forceConvert := strings.ToLower(settings.ForceConvert)
	blockPaths := settings.BlockPaths
	blockFlags := settings.BlockFlags
	useMarkov := settings.UseMarkov
	settingsMutex.RUnlock()

	var wordData []KeyEventCore
	var deletions int

	if isManual {
		if len(currentWord) > 0 {
			wordData = make([]KeyEventCore, len(currentWord))
			copy(wordData, currentWord)
			deletions = len(wordData)
			currentWord = currentWord[:0]
			lastWord = make([]KeyEventCore, len(wordData))
			copy(lastWord, wordData)
			typedSpaceAfterLastWord = false
		} else if len(lastWord) > 0 {
			wordData = make([]KeyEventCore, len(lastWord))
			copy(wordData, lastWord)
			deletions = len(wordData)
			if typedSpaceAfterLastWord {
				deletions++
				appendChar = " "
			}
		} else {
			return
		}
	} else {
		if len(currentWord) == 0 {
			return
		}
		wordData = make([]KeyEventCore, len(currentWord))
		copy(wordData, currentWord)
		deletions = len(wordData)
		lastWord = make([]KeyEventCore, len(wordData))
		copy(lastWord, wordData)
		typedSpaceAfterLastWord = (appendChar != "")
		currentWord = currentWord[:0]
		if !aiEnabled {
			return
		}
	}

	wordEn, wordRu, hasUpper, hasLower, hasDigit, hasCapsLock := buildWordStrings(wordData)
	wLowerEn, wLowerRu := strings.ToLower(wordEn), strings.ToLower(wordRu)

	// Очищаем для проверки исключений
	cEn, cRu := cleanWord(wordEn, "en"), cleanWord(wordRu, "ru")
	isRu := osIsRussianLayout()

	if isRu {
		lastRenderedWord = wordRu
	} else {
		lastRenderedWord = wordEn
	}
	lastRenderedSuffix = appendChar

	for _, excLine := range strings.Split(exceptions, "\n") {
		excLine = strings.TrimSpace(excLine)
		if excLine == "" {
			continue
		}
		if wLowerEn == excLine || wLowerRu == excLine || cEn == excLine || cRu == excLine {
			return
		}
	}

	if isManual {
		if isRu {
			learnForceConvert(cRu)
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, appendChar, true)
		} else {
			learnForceConvert(cEn)
			lastFinalLang = "ru"
			go replaceText(deletions, wordRu, appendChar, true)
		}
		return
	}

	for _, fcLine := range strings.Split(forceConvert, "\n") {
		fcLine = strings.TrimSpace(fcLine)
		if fcLine == "" {
			continue
		}
		if cEn == fcLine && !isRu {
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
				Corrected: wordRu, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "ru"
			go replaceText(deletions, wordRu, appendChar, true)
			return
		}
		if cRu == fcLine && isRu {
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "en"
			go replaceText(deletions, wordEn, appendChar, true)
			return
		}
	}

	if isITPattern(wordEn) {
		if isRu {
			lastFinalLang = "en"
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			go replaceText(deletions, wordEn, appendChar, true)
		} else {
			lastFinalLang = "en"
		}
		return
	}

	if !isManual {
		if hasDigit || (hasUpper && hasLower && !hasCapsLock) {
			return
		}
	}

	isFlag := strings.HasPrefix(wordEn, "-")
	isPath := strings.HasPrefix(wordEn, "\\") || strings.HasPrefix(wordEn, "/")
	if (isFlag && blockFlags) || (isPath && blockPaths) {
		if isRu {
			lastFinalLang = "en"
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: wordEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			go replaceText(deletions, wordEn, appendChar, true)
		} else {
			lastFinalLang = "en"
		}
		return
	}

	adjustedThreshold := threshold
	if useMarkov {
		if isRu && lastFinalLang == "en" {
			adjustedThreshold -= 0.15
		} else if !isRu && lastFinalLang == "ru" {
			adjustedThreshold -= 0.15
		}
	}

	if len(wordEn) >= 3 {
		dist := CalcAverageJump(wLowerEn)
		visualWord := cEn
		if isRu {
			visualWord = cRu
		}
		addTopoRecord(visualWord, dist)
	}

	// ⚡ Передаем СЫРЫЕ слова (wordRu, wordEn) в ShouldConvertWord
	if isRu {
		if ShouldConvertWord(wordRu, wordEn, "ru", lastFinalLang, adjustedThreshold) {
			finalEn := wordEn
			if hasCapsLock {
				finalEn = fixInvertedCaps(finalEn)
			}
			correctedEn, _ := ApplyAutoCorrect(finalEn, "en")
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
				Corrected: correctedEn, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cRu)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "en"
			go replaceText(deletions, correctedEn, appendChar, true)
		} else {
			lastFinalLang = "ru"
			if correctedRu, changed := ApplyAutoCorrect(wordRu, "ru"); changed {
				lastAutoCorrection = &CorrectionData{
					OriginalFull: wordRu, OriginalClean: cRu, PhysicalEn: wLowerEn,
					Corrected: correctedRu, AppendChar: appendChar, SwitchedLang: false, Timestamp: time.Now(),
				}
				incrementTypoCorrection(len([]rune(cRu)))
				go replaceText(deletions, correctedRu, appendChar, false)
			}
		}
	} else {
		if ShouldConvertWord(wordEn, wordRu, "en", lastFinalLang, adjustedThreshold) {
			finalRu := wordRu
			if hasCapsLock {
				finalRu = fixInvertedCaps(finalRu)
			}
			correctedRu, _ := ApplyAutoCorrect(finalRu, "ru")
			lastAutoCorrection = &CorrectionData{
				OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
				Corrected: correctedRu, AppendChar: appendChar, SwitchedLang: true, Timestamp: time.Now(),
			}
			incrementLayoutCorrection(len([]rune(cEn)))
			recordHeatmapError(wLowerEn)
			lastFinalLang = "ru"
			go replaceText(deletions, correctedRu, appendChar, true)
		} else {
			lastFinalLang = "en"
			if correctedEn, changed := ApplyAutoCorrect(wordEn, "en"); changed {
				lastAutoCorrection = &CorrectionData{
					OriginalFull: wordEn, OriginalClean: cEn, PhysicalEn: wLowerEn,
					Corrected: correctedEn, AppendChar: appendChar, SwitchedLang: false, Timestamp: time.Now(),
				}
				incrementTypoCorrection(len([]rune(cEn)))
				go replaceText(deletions, correctedEn, appendChar, false)
			}
		}
	}
}

func EngineResetCounters() {
	lastShiftTime = time.Time{}
	backspaceCount = 0
}
