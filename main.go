package main

import (
	"embed"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
)

//go:embed web/index.html
var htmlContent embed.FS

//go:embed icon.ico
var iconData []byte

type AppSettings struct {
	EnableAI   bool   `json:"enable_ai"`
	Threshold  int    `json:"threshold"`
	Hotkey     string `json:"hotkey"`
	Exceptions string `json:"exceptions"`
	Autostart  bool   `json:"autostart"`
}

var (
	settings      AppSettings
	settingsMutex sync.RWMutex
)

func enforceSingleInstance() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex := kernel32.NewProc("CreateMutexW")

	mutexName, _ := syscall.UTF16PtrFromString("GoSwitcher_Unique_Mutex")
	handle, _, err := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))

	if handle == 0 || err == syscall.Errno(183) {
		os.Exit(0)
	}
}

func getAppDir() string {
	exePath, _ := os.Executable()
	return filepath.Dir(exePath)
}

func logDebug(format string, v ...interface{}) {
	// Релизный режим: логирование отключено
}

func loadSettings() {
	settings = AppSettings{
		EnableAI:   true,
		Threshold:  80,
		Hotkey:     "pause",
		Exceptions: "",
		Autostart:  false,
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

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
	if err == nil {
		defer k.Close()
		exe, _ := os.Executable()
		if newSettings.Autostart {
			k.SetStringValue("GoSwitcher", `"`+exe+`"`)
		} else {
			k.DeleteValue("GoSwitcher")
		}
	}
}

func main() {
	enforceSingleInstance()

	learnFlag := flag.Bool("learn", false, "Сгенерировать model.json из txt файлов")
	flag.Parse()

	if *learnFlag {
		TrainModel("words_ru.txt", "words_en.txt")
		os.Exit(0)
	}

	loadSettings()
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
		// ФИКС: Используем switch вместо if/else для проверки r.Method
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
				exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://127.0.0.1:8765").Start()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	os.Exit(0)
}
