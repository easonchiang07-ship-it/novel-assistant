package setup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ProgressFn receives install progress: percent 0–100 and a human-readable message.
// percent == -1 means indeterminate.
type ProgressFn func(percent int, msg string)

// IsOllamaInstalled returns true if the ollama binary is findable.
func IsOllamaInstalled() bool {
	if _, err := exec.LookPath("ollama"); err == nil {
		return true
	}
	// Windows: Ollama may not be on PATH right after a silent install.
	if runtime.GOOS == "windows" {
		candidates := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Ollama", "ollama.exe"),
			`C:\Program Files\Ollama\ollama.exe`,
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

// InstallOllama downloads and silently installs Ollama for the current OS.
func InstallOllama(progress ProgressFn) error {
	switch runtime.GOOS {
	case "windows":
		return installWindows(progress)
	case "darwin":
		return installDarwin(progress)
	case "linux":
		return installLinux(progress)
	default:
		return fmt.Errorf("不支援的作業系統：%s", runtime.GOOS)
	}
}

func installWindows(progress ProgressFn) error {
	url := "https://ollama.com/download/OllamaSetup.exe"
	tmp := filepath.Join(os.TempDir(), "OllamaSetup.exe")
	defer os.Remove(tmp) //nolint:errcheck

	progress(5, "正在下載 Ollama 安裝程式...")
	if err := downloadWithProgress(url, tmp, 5, 70, progress); err != nil {
		return err
	}

	progress(75, "正在安裝 Ollama（背景靜默執行）...")
	if out, err := exec.Command(tmp, "/S").CombinedOutput(); err != nil { // #nosec G204 -- tmp is the downloaded Ollama installer from a hardcoded URL
		return fmt.Errorf("安裝失敗：%w\n%s", err, out)
	}

	progress(90, "等待 Ollama 服務啟動...")
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		if IsOllamaInstalled() {
			break
		}
	}
	progress(100, "Ollama 安裝完成 ✓")
	return nil
}

func installDarwin(progress ProgressFn) error {
	url := "https://ollama.com/download/Ollama-darwin.zip"
	tmp := filepath.Join(os.TempDir(), "Ollama-darwin.zip")
	defer os.Remove(tmp) //nolint:errcheck

	progress(5, "正在下載 Ollama...")
	if err := downloadWithProgress(url, tmp, 5, 65, progress); err != nil {
		return err
	}

	extractDir := filepath.Join(os.TempDir(), "ollama-extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("無法建立暫存目錄：%w", err)
	}
	defer os.RemoveAll(extractDir) //nolint:errcheck

	progress(70, "正在解壓縮...")
	if out, err := exec.Command("unzip", "-o", tmp, "-d", extractDir).CombinedOutput(); err != nil { // #nosec G204 -- unzip with controlled args from temp dir
		return fmt.Errorf("解壓縮失敗：%w\n%s", err, out)
	}

	progress(85, "正在安裝至 Applications...")
	src := filepath.Join(extractDir, "Ollama.app")
	if out, err := exec.Command("cp", "-rf", src, "/Applications/Ollama.app").CombinedOutput(); err != nil { // #nosec G204 -- cp with controlled src from temp extract dir
		return fmt.Errorf("複製失敗：%w\n%s", err, out)
	}

	progress(95, "正在啟動 Ollama...")
	exec.Command("open", "/Applications/Ollama.app").Start() //nolint:errcheck
	time.Sleep(3 * time.Second)

	progress(100, "Ollama 安裝完成 ✓")
	return nil
}

func installLinux(progress ProgressFn) error {
	progress(10, "正在安裝 Ollama（需要 sudo 權限）...")
	cmd := exec.Command("bash", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("安裝失敗：%w", err)
	}
	progress(100, "Ollama 安裝完成 ✓")
	return nil
}

// downloadWithProgress downloads url to dest, reporting progress between startPct and endPct.
func downloadWithProgress(url, dest string, startPct, endPct int, progress ProgressFn) error {
	resp, err := http.Get(url) // #nosec G107 -- URL is a hardcoded constant (Ollama download endpoint)
	if err != nil {
		return fmt.Errorf("下載失敗：%w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下載失敗：伺服器回應 %d", resp.StatusCode)
	}

	f, err := os.Create(dest) // #nosec G304 -- dest is os.TempDir() + hardcoded filename, not user input
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("寫入失敗：%w", werr)
			}
			downloaded += int64(n)
			if total > 0 {
				pct := startPct + int(float64(downloaded)/float64(total)*float64(endPct-startPct))
				progress(pct, fmt.Sprintf("已下載 %.1f / %.1f MB",
					float64(downloaded)/1024/1024, float64(total)/1024/1024))
			} else {
				progress(-1, fmt.Sprintf("已下載 %.1f MB", float64(downloaded)/1024/1024))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ParseOllamaPullLine extracts a percent value from an ollama pull output line.
// Returns -1 when the line carries no percentage.
func ParseOllamaPullLine(line string) int {
	line = strings.TrimSpace(line)
	if idx := strings.Index(line, "%"); idx > 0 {
		start := strings.LastIndex(line[:idx], " ")
		numStr := strings.TrimSpace(line[start+1 : idx])
		var pct int
		if _, err := fmt.Sscanf(numStr, "%d", &pct); err == nil {
			return pct
		}
	}
	return -1
}
