package deps

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Tool defines an external dependency with install instructions
type Tool struct {
	Name     string   // display name
	Bin      string   // binary to check via exec.LookPath
	Required bool     // if true, startup will fail without it
	Install  []string // auto-install commands (first successful one wins)
}

// Status represents the availability of a tool
type Status struct {
	Name      string `json:"name"`
	Bin       string `json:"bin"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Installed bool   `json:"installed"` // was just auto-installed
	Error     string `json:"error,omitempty"`
}

// Predefined tools
var Tools = []Tool{
	{
		Name: "FFmpeg", Bin: "ffmpeg", Required: true,
		Install: installCommands("ffmpeg"),
	},
	{
		Name: "pdf-inspector", Bin: "pdf2md", Required: false,
		Install: []string{"npm install -g @firecrawl/pdf-inspector"},
	},
	{
		Name: "epub2md", Bin: "epub2md", Required: false,
		Install: []string{"npm install -g epub2md"},
	},
	{
		Name: "opendataloader-pdf", Bin: "opendataloader-pdf", Required: false,
		Install: []string{"pip install opendataloader-pdf", "pip3 install opendataloader-pdf"},
	},
	{
		Name: "Pandoc", Bin: "pandoc", Required: false,
		Install: installCommands("pandoc"),
	},
	{
		Name: "Calibre (ebook-convert)", Bin: "ebook-convert", Required: false,
		Install: installCommands("calibre"),
	},
	{
		Name: "pdftotext", Bin: "pdftotext", Required: false,
		Install: installCommands("poppler-utils"),
	},
}

// installCommands returns OS-appropriate install commands for a package
func installCommands(pkg string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{fmt.Sprintf("brew install %s", pkg)}
	case "linux":
		// Try apt first, then dnf/yum, then pacman
		return []string{
			fmt.Sprintf("sudo apt-get install -y %s", pkg),
			fmt.Sprintf("sudo dnf install -y %s", pkg),
			fmt.Sprintf("sudo pacman -S --noconfirm %s", pkg),
		}
	default:
		return []string{fmt.Sprintf("choco install %s", pkg)}
	}
}

// CheckResult holds the results of checking all tools
type CheckResult struct {
	AllAvailable bool     `json:"all_available"`
	Tools        []Status `json:"tools"`
}

// CheckAll checks all tools and returns their status.
// If autoInstall is true, tries to install missing tools.
func CheckAll(autoInstall bool) *CheckResult {
	result := &CheckResult{AllAvailable: true}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tool := range Tools {
		wg.Add(1)
		go func(t Tool) {
			defer wg.Done()
			status := checkOne(t, autoInstall)
			mu.Lock()
			result.Tools = append(result.Tools, status)
			if !status.Available && t.Required {
				result.AllAvailable = false
			}
			mu.Unlock()
		}(tool)
	}
	wg.Wait()
	return result
}

func checkOne(tool Tool, autoInstall bool) Status {
	status := Status{Name: tool.Name, Bin: tool.Bin}

	path, err := exec.LookPath(tool.Bin)
	if err == nil {
		status.Available = true
		status.Path = path
		return status
	}

	// Not found — try auto-install
	if autoInstall && len(tool.Install) > 0 {
		log.Printf("🔧 %s not found, attempting auto-install...", tool.Name)
		for _, cmdStr := range tool.Install {
			log.Printf("   $ %s", cmdStr)
			parts := strings.Fields(cmdStr)
			if len(parts) == 0 {
				continue
			}
			cmd := exec.Command(parts[0], parts[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("   ⚠️  Install failed: %v", err)
				continue
			}
			// Re-check after install
			if path, err := exec.LookPath(tool.Bin); err == nil {
				status.Available = true
				status.Path = path
				status.Installed = true
				log.Printf("   ✅ %s installed successfully at %s", tool.Name, path)
				return status
			}
		}
	}

	status.Error = fmt.Sprintf("%s not found: %v", tool.Bin, err)
	if !tool.Required {
		status.Error += " (optional, some features will use fallbacks)"
	}
	return status
}

// LogReport prints a human-readable dependency report
func LogReport(result *CheckResult) {
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("📦 Dependency Check")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for _, s := range result.Tools {
		icon := "✅"
		if s.Installed {
			icon = "🔧"
		} else if !s.Available {
			icon = "⚠️"
		}
		detail := s.Path
		if !s.Available {
			detail = s.Error
		}
		log.Printf("  %s %-30s %s", icon, s.Name, detail)
	}

	if !result.AllAvailable {
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("⚠️  Some REQUIRED dependencies are missing.")
		log.Println("   Install them manually or restart with auto-install enabled.")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	} else {
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	}
}
