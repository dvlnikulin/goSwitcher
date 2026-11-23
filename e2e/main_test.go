//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// switcherProcess хранит запущенный процесс свитчера
var switcherProcess *exec.Cmd
var switcherBin string
var projectRoot string

// TestMain компилирует и запускает свитчер перед прогоном тестов,
// гарантированно убивает процесс после завершения.
func TestMain(m *testing.M) {
	// Определяем корень проекта
	var err error
	projectRoot, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getwd: %v\n", err)
		os.Exit(1)
	}
	// Поднимаемся на уровень выше (e2e/ находится в proj/e2e/)
	projectRoot = filepath.Dir(projectRoot)

	// Создаём временную директорию для бинарника
	tmpDir, err := os.MkdirTemp("", "goswitcher-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "MkdirTemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	switcherBin = filepath.Join(tmpDir, "goswitcher-e2e.exe")

	// Компилируем свитчер
	fmt.Printf("[E2E] Компиляция свитчера: %s\n", switcherBin)
	buildCmd := exec.Command("go", "build", "-mod=mod", "-o", switcherBin, ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[E2E] Ошибка сборки: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[E2E] Сборка успешна")

	// Запускаем свитчер
	fmt.Println("[E2E] Запуск свитчера...")
	switcherProcess = exec.Command(switcherBin)
	switcherProcess.Dir = projectRoot
	switcherProcess.Stdout = os.Stdout
	switcherProcess.Stderr = os.Stderr
	// Используем StartDetached на Windows для изоляции
	switcherProcess.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NEW_PROCESS_GROUP
	if err := switcherProcess.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[E2E] Ошибка запуска: %v\n", err)
		os.Exit(1)
	}
	// Pid доступен после Start()
	fmt.Printf("[E2E] PID: %d\n", switcherProcess.Process.Pid)

	// Ждём готовности веб-сервера
	if !waitForServer(15*time.Second) {
		fmt.Fprintln(os.Stderr, "[E2E] Свитчер не запустился (timeout)")
		killSwitcher()
		os.Exit(1)
	}
	fmt.Println("[E2E] Свитчер готов")

	// Запускаем тесты
	code := m.Run()

	// Гарантированная остановка
	killSwitcher()
	fmt.Println("[E2E] Свитчер остановлен")

	os.Exit(code)
}

// waitForServer ждёт пока веб-сервер свитчера ответит на /api/stats
func waitForServer(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:8765/api/stats")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// killSwitcher завершает процесс свитчера
func killSwitcher() {
	if switcherProcess == nil || switcherProcess.Process == nil {
		return
	}
	fmt.Println("[E2E] Остановка свитчера...")
	switcherProcess.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- switcherProcess.Wait()
	}()
	select {
	case <-done:
		fmt.Println("[E2E] Свитчер остановлен gracefully")
	case <-time.After(5 * time.Second):
		fmt.Println("[E2E] Принудительное завершение...")
		switcherProcess.Process.Kill()
		<-done
	}
}

// cleanupSwitcher гарантирует однократное завершение процесса
var once sync.Once

func cleanupSwitcher() {
	once.Do(func() {
		killSwitcher()
	})
}

// ─────────────────────────────────────────────────────────────
// Утилиты для печати на русском (UTF-8 в консоли Windows)
// ─────────────────────────────────────────────────────────────

func printStep(msg string) {
	fmt.Printf("  [E2E] %s\n", msg)
}
