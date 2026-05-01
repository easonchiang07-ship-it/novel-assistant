package setup

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemSpecs holds detected hardware information.
type SystemSpecs struct {
	RAMGB    float64 `json:"ram_gb"`
	CPUCores int     `json:"cpu_cores"`
	GPUName  string  `json:"gpu_name"`
	VRAMGB   float64 `json:"vram_gb"`
	OS       string  `json:"os"`
	Arch     string  `json:"arch"`
}

// DetectSpecs reads RAM, CPU, and GPU information from the current machine.
func DetectSpecs() SystemSpecs {
	return SystemSpecs{
		RAMGB:    detectRAMGB(),
		CPUCores: runtime.NumCPU(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		GPUName:  detectGPUName(),
		VRAMGB:   detectVRAMGB(),
	}
}

func detectRAMGB() float64 {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("grep", "MemTotal", "/proc/meminfo").Output()
		if err != nil {
			return 0
		}
		fields := strings.Fields(string(out))
		if len(fields) >= 2 {
			kb, _ := strconv.ParseFloat(fields[1], 64)
			return roundGB(kb / 1024 / 1024)
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		b, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return roundGB(b / 1024 / 1024 / 1024)
	case "windows":
		// Try wmic first (available on Windows 10 and older).
		if gb := windowsRAMViaWmic(); gb > 0 {
			return gb
		}
		// Fallback: PowerShell Get-CimInstance (available when wmic is absent).
		return windowsRAMViaPowerShell()
	}
	return 0
}

func windowsRAMViaWmic() float64 {
	out, err := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TotalPhysicalMemory=") {
			val := strings.TrimPrefix(line, "TotalPhysicalMemory=")
			b, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
			return roundGB(b / 1024 / 1024 / 1024)
		}
	}
	return 0
}

func windowsRAMViaPowerShell() float64 {
	out, err := exec.Command(
		"powershell", "-NoProfile", "-NonInteractive", "-Command",
		"(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory",
	).Output()
	if err != nil {
		return 0
	}
	b, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return roundGB(b / 1024 / 1024 / 1024)
}

func detectGPUName() string {
	// NVIDIA
	if out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output(); err == nil {
		name := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		if name != "" {
			return name
		}
	}
	// macOS — Apple Silicon or discrete GPU
	if runtime.GOOS == "darwin" {
		out, _ := exec.Command("system_profiler", "SPDisplaysDataType").Output()
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Chipset Model:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
			}
		}
	}
	return ""
}

func detectVRAMGB() float64 {
	// NVIDIA
	if out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits").Output(); err == nil {
		mb, _ := strconv.ParseFloat(strings.TrimSpace(strings.Split(string(out), "\n")[0]), 64)
		if mb > 0 {
			return roundGB(mb / 1024)
		}
	}
	// macOS — unified memory reported as VRAM
	if runtime.GOOS == "darwin" {
		out, _ := exec.Command("system_profiler", "SPDisplaysDataType").Output()
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "VRAM") && strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				fields := strings.Fields(parts[1])
				if len(fields) >= 1 {
					mb, _ := strconv.ParseFloat(fields[0], 64)
					unit := ""
					if len(fields) >= 2 {
						unit = strings.ToUpper(fields[1])
					}
					if unit == "GB" {
						return roundGB(mb)
					}
					if mb > 0 {
						return roundGB(mb / 1024)
					}
				}
			}
		}
	}
	return 0
}

func roundGB(v float64) float64 {
	// round to one decimal
	return float64(int(v*10+0.5)) / 10
}
