// Package platform detects the runtime environment and available capabilities.
// All platform-specific branching in the project should go through this package.
package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// Info describes the current runtime environment and what it can do.
type Info struct {
	OS       string // runtime.GOOS: "windows", "linux", "darwin", "android", "ios"
	Arch     string // runtime.GOARCH: "amd64", "arm", "arm64", etc.
	IsPI     bool   // true if running on Raspberry Pi (arm/arm64 linux)
	IsMobile bool   // true if Android or iOS — no subprocess, limited filesystem
	CanExec  bool   // can spawn subprocesses (false on iOS/Android)
	HasTTY   bool   // stdin is an interactive terminal

	// Available external tools — empty string means not found.
	DirewolfPath string
	RigctldPath  string

	// Preferred ham audio backend for this platform.
	// "direwolf" if Direwolf is available and CanExec.
	// "audio"    if using built-in malgo AFSK modem.
	// "none"     if no audio capability detected.
	AudioBackend string
}

// Detect probes the current system and returns a populated Info.
// Cheap to call; does file-stat and exec lookups but no network or audio init.
func Detect() *Info {
	p := &Info{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	p.IsMobile = p.OS == "android" || p.OS == "ios"
	p.CanExec = !p.IsMobile
	p.HasTTY = term.IsTerminal(int(os.Stdin.Fd()))

	if p.OS == "linux" {
		p.IsPI = isRaspberryPi()
	}

	if p.CanExec {
		p.DirewolfPath = findBinary("direwolf", direwolfCandidates(p.OS))
		p.RigctldPath = findBinary("rigctld", rigctldCandidates(p.OS))
	}

	switch {
	case p.DirewolfPath != "" && p.CanExec:
		p.AudioBackend = "direwolf"
	case !p.IsMobile:
		p.AudioBackend = "audio"
	default:
		p.AudioBackend = "none"
	}

	return p
}

// isRaspberryPi checks /proc/cpuinfo for the Pi hardware signature.
func isRaspberryPi() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "raspberry pi") || strings.Contains(s, "bcm2")
}

// findBinary returns the full path to name if found via env override, PATH, or candidates.
// Env var checked: uppercase name with hyphens replaced by underscores + "_PATH"
// e.g. "direwolf" → DIREWOLF_PATH, "rigctld" → RIGCTLD_PATH
// Returns "" if not found.
func findBinary(name string, candidates []string) string {
	envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_PATH"
	if path := os.Getenv(envKey); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func direwolfCandidates(goos string) []string {
	switch goos {
	case "windows":
		return []string{
			`C:\Program Files\Direwolf\direwolf.exe`,
			`C:\Program Files (x86)\Direwolf\direwolf.exe`,
			`C:\direwolf\direwolf.exe`,
		}
	case "darwin":
		return []string{
			"/usr/local/bin/direwolf",
			"/opt/homebrew/bin/direwolf",
		}
	default: // linux, pi
		return []string{
			"/usr/bin/direwolf",
			"/usr/local/bin/direwolf",
		}
	}
}

func rigctldCandidates(goos string) []string {
	switch goos {
	case "windows":
		return []string{
			`C:\Program Files\Hamlib\bin\rigctld.exe`,
			`C:\hamlib\bin\rigctld.exe`,
		}
	case "darwin":
		return []string{
			"/usr/local/bin/rigctld",
			"/opt/homebrew/bin/rigctld",
		}
	default:
		return []string{
			"/usr/bin/rigctld",
			"/usr/local/bin/rigctld",
		}
	}
}
