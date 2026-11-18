package main

import (
	"embed"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/getlantern/systray"
)

//go:embed web/index.html
var htmlContent embed.FS

//go:embed icon.ico
var iconData []byte

type AppSettings struct {
	EnableAI        bool   `json:"enable_ai"`
	Threshold       int    `json:"threshold"`
	Hotkey          string `json:"hotkey"`
	CaseHotkey      string `json:"case_hotkey"`
	Exceptions      string `json:"exceptions"`
	ForceConvert    string `json:"force_convert"`
	Blacklist       string `json:"blacklist"`
	Autostart       bool   `json:"autostart"`
	CustomVocab     string `json:"custom_vocab"`
	BlockPaths      bool   `json:"block_paths"`
	BlockFlags      bool   `json:"block_flags"`
	UseMarkov       bool   `json:"use_markov"`
	LiveTypoEnabled bool   `json:"live_typo_enabled"`
	LiveTypoChar    int    `json:"live_typo_char"`
	TrainingLimit   int    `json:"training_limit"`
	LaplacePenalty  int    `json:"laplace_penalty"`
}

type AppStats struct {
	LayoutCorrections int `json:"layout_corrections"`
	TypoCorrections   int `json:"typo_corrections"`
	SavedKeystrokes   int `json:"saved_keystrokes"`
	UndoCount         int `json:"undo_count"`
}

type TopoRecord struct {
	Word     string  `json:"word"`
	Distance float64 `json:"distance"`
}

var (
	settings      AppSettings
	settingsMutex sync.RWMutex
	stats         AppStats
	statsMutex    sync.RWMutex
	topoHistory   []TopoRecord
	topoMutex     sync.RWMutex
	keyHeatmap    map[string]int
	heatmapMutex  sync.RWMutex
)

func getAppDir() string {
	exePath, _ := os.Executable()
	return filepath.Dir(exePath)
}

func logDebug(format string, v ...interface{}) {}

func loadSettings() {
	settings = AppSettings{
		EnableAI:        true,
		Threshold:       80,
		Hotkey:          "pause",
		CaseHotkey:      "shift+pause",
		Exceptions:      "",
		ForceConvert:    "",
		Blacklist:       defaultBlacklist(),
		Autostart:       false,
		CustomVocab:     "kubectl\nmikrotik\nrouteros\nwinbox\nansible\ndocker\nnginx",
		BlockPaths:      true,
		BlockFlags:      true,
		UseMarkov:       true,
		LiveTypoEnabled: true,
		LiveTypoChar:    4,
		TrainingLimit:   60000,
		LaplacePenalty:  1,
	}
	data, err := os.ReadFile(filepath.Join(getAppDir(), "settings.json"))
	if err == nil {
		json.Unmarshal(data, &settings)
	}
}

func saveSettings(newSettings AppSettings) {
	settingsMutex.Lock()
	settings = newSettings
	settingsMutex.Unlock()

	data, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(getAppDir(), "settings.json"), data, 0644)

	setSystemAutostart(newSettings.Autostart)
}

func loadStats() {
	data, err := os.ReadFile(filepath.Join(getAppDir(), "stats.json"))
	if err == nil {
		json.Unmarshal(data, &stats)
	}
}

func saveStats() {
	statsMutex.RLock()
	data, _ := json.MarshalIndent(stats, "", "  ")
	statsMutex.RUnlock()
	os.WriteFile(filepath.Join(getAppDir(), "stats.json"), data, 0644)
}

func loadHeatmap() {
	keyHeatmap = make(map[string]int)
	data, err := os.ReadFile(filepath.Join(getAppDir(), "heatmap.json"))
	if err == nil {
		json.Unmarshal(data, &keyHeatmap)
	}
}

func saveHeatmap() {
	heatmapMutex.RLock()
	data, _ := json.MarshalIndent(keyHeatmap, "", "  ")
	heatmapMutex.RUnlock()
	os.WriteFile(filepath.Join(getAppDir(), "heatmap.json"), data, 0644)
}

func recordHeatmapError(physicalWordEn string) {
	if len(physicalWordEn) == 0 {
		return
	}
	heatmapMutex.Lock()
	if keyHeatmap == nil {
		keyHeatmap = make(map[string]int)
	}
	for _, r := range physicalWordEn {
		if r >= 'a' && r <= 'z' || r == ';' || r == '\'' || r == ',' || r == '.' || r == '[' || r == ']' {
			keyHeatmap[string(r)]++
		}
	}
	heatmapMutex.Unlock()
	go saveHeatmap()
}

func revertHeatmapError(physicalWordEn string) {
	if len(physicalWordEn) == 0 {
		return
	}
	heatmapMutex.Lock()
	if keyHeatmap != nil {
		for _, r := range physicalWordEn {
			k := string(r)
			if keyHeatmap[k] > 0 {
				keyHeatmap[k]--
			}
		}
	}
	heatmapMutex.Unlock()
	go saveHeatmap()
}

func incrementLayoutCorrection(wordLength int) {
	statsMutex.Lock()
	stats.LayoutCorrections++
	stats.SavedKeystrokes += wordLength * 2
	statsMutex.Unlock()
	go saveStats()
}

func incrementTypoCorrection(wordLength int) {
	statsMutex.Lock()
	stats.TypoCorrections++
	stats.SavedKeystrokes += wordLength
	statsMutex.Unlock()
	go saveStats()
}

func incrementUndoCount(wordLength int, switchedLang bool) {
	statsMutex.Lock()
	stats.UndoCount++
	if switchedLang {
		stats.SavedKeystrokes -= wordLength * 2
	} else {
		stats.SavedKeystrokes -= wordLength
	}
	if stats.SavedKeystrokes < 0 {
		stats.SavedKeystrokes = 0
	}
	if switchedLang && stats.LayoutCorrections > 0 {
		stats.LayoutCorrections--
	} else if !switchedLang && stats.TypoCorrections > 0 {
		stats.TypoCorrections--
	}
	statsMutex.Unlock()
	go saveStats()
}

func addTopoRecord(word string, dist float64) {
	topoMutex.Lock()
	defer topoMutex.Unlock()
	topoHistory = append([]TopoRecord{{Word: word, Distance: dist}}, topoHistory...)
	if len(topoHistory) > 12 {
		topoHistory = topoHistory[:12]
	}
}

func main() {
	learnFlag := flag.Bool("learn", false, "Сгенерировать model.json из txt файлов")
	flag.Parse()

	if *learnFlag {
		TrainModel("words_ru.txt", "words_en.txt")
		os.Exit(0)
	}

	enforceSingleInstance()
	loadSettings()
	loadStats()
	loadHeatmap()
	loadModel()

	go startWebServer()
	go startHook()

	systray.Run(onReady, onExit)
}

func startWebServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := htmlContent.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	http.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsMutex.RLock()
			defer settingsMutex.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(settings)
		case http.MethodPost:
			var s AppSettings
			json.NewDecoder(r.Body).Decode(&s)
			saveSettings(s)
			w.WriteHeader(http.StatusOK)
		}
	})

	http.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		statsMutex.RLock()
		currentStats := stats
		statsMutex.RUnlock()

		topoMutex.RLock()
		currentTopo := make([]TopoRecord, len(topoHistory))
		copy(currentTopo, topoHistory)
		topoMutex.RUnlock()

		heatmapMutex.RLock()
		currentHeat := make(map[string]int, len(keyHeatmap))
		for k, v := range keyHeatmap {
			currentHeat[k] = v
		}
		heatmapMutex.RUnlock()

		response := struct {
			LayoutCorrections int            `json:"layout_corrections"`
			TypoCorrections   int            `json:"typo_corrections"`
			SavedKeystrokes   int            `json:"saved_keystrokes"`
			UndoCount         int            `json:"undo_count"`
			Topography        []TopoRecord   `json:"topography"`
			Heatmap           map[string]int `json:"heatmap"`
		}{
			LayoutCorrections: currentStats.LayoutCorrections,
			TypoCorrections:   currentStats.TypoCorrections,
			SavedKeystrokes:   currentStats.SavedKeystrokes,
			UndoCount:         currentStats.UndoCount,
			Topography:        currentTopo,
			Heatmap:           currentHeat,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/api/retrain", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := TrainModel("words_ru.txt", "words_en.txt")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			loadModel()
			w.WriteHeader(http.StatusOK)
		}
	})

	http.HandleFunc("/api/reset_heatmap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			heatmapMutex.Lock()
			keyHeatmap = make(map[string]int)
			heatmapMutex.Unlock()
			go saveHeatmap()
			w.WriteHeader(http.StatusOK)
		}
	})

	http.ListenAndServe("127.0.0.1:8765", nil)
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("GoSwitcher")
	systray.SetTooltip("ИИ Автозамена")

	mSettings := systray.AddMenuItem("Настройки", "Открыть настройки в браузере")
	mQuit := systray.AddMenuItem("Выход", "Закрыть программу")

	go func() {
		for {
			select {
			case <-mSettings.ClickedCh:
				openBrowser("http://127.0.0.1:8765")
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	os.Exit(0)
}
