package ham

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/platform"
)

// LaunchDirewolf starts a Direwolf subprocess and connects to its KISS TCP port.
//
// binaryHint: path to direwolf binary; empty = search PATH + common locations.
// configPath: path to direwolf.conf; empty = use "direwolf.conf" in cwd.
// If no config exists and we have a TTY, runs the interactive setup wizard.
// If no config exists and no TTY (headless), returns an error.
//
// Fails fast if Direwolf is already listening on the KISS port.
func LaunchDirewolf(configPath string, kissPort int) (*DirewolfDevice, error) {
	return LaunchDirewolfWithBinary("", configPath, kissPort)
}

// LaunchDirewolfWithBinary is like LaunchDirewolf but accepts an explicit binary path.
func LaunchDirewolfWithBinary(binaryHint, configPath string, kissPort int) (*DirewolfDevice, error) {
	if kissPort == 0 {
		kissPort = 8001
	}
	if configPath == "" {
		configPath = "direwolf.conf"
	}

	binary := binaryHint
	if binary == "" {
		p := platform.Detect()
		if !p.CanExec {
			return nil, fmt.Errorf("direwolf: cannot spawn subprocesses on %s", p.OS)
		}
		if p.DirewolfPath == "" {
			return nil, fmt.Errorf("direwolf binary not found on PATH or common install locations; set direwolf_path in config")
		}
		binary = p.DirewolfPath
	} else if _, err := os.Stat(binary); err != nil {
		return nil, fmt.Errorf("direwolf binary not found at configured path %q", binary)
	}

	// Ensure config exists.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if !platform.Detect().HasTTY {
			return nil, fmt.Errorf("direwolf: config %q not found and no TTY for interactive setup (headless requires config)", configPath)
		}
		fmt.Printf("No Direwolf config found at %q. Running setup wizard...\n\n", configPath)
		if err := RunDirewolfSetup(configPath, kissPort); err != nil {
			return nil, fmt.Errorf("direwolf setup: %w", err)
		}
	}

	// Start Direwolf.
	// -t 0: no text colors (cleaner logs)
	cmd := exec.Command(filepath.Clean(binary), "-t", "0", "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("direwolf: failed to start process: %w", err)
	}

	// Wait for KISS port to be ready (up to 10s).
	addr := fmt.Sprintf("127.0.0.1:%d", kissPort)
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			return nil, fmt.Errorf("direwolf: timed out waiting for KISS port %d", kissPort)
		}
		// Check if process died already.
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return nil, fmt.Errorf("direwolf: process exited before KISS port became ready")
		}
		time.Sleep(200 * time.Millisecond)
	}

	d, err := ConnectDirewolf("127.0.0.1", kissPort)
	if err != nil {
		cmd.Process.Kill()
		return nil, err
	}
	d.cmd = cmd
	return d, nil
}

// KillAllDirewolf kills any running Direwolf processes and waits for ports to release.
// Used by tests and CLI to ensure a clean slate before launching.
func KillAllDirewolf() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("taskkill", "/F", "/IM", "direwolf.exe")
	} else {
		cmd = exec.Command("pkill", "-f", "direwolf")
	}
	_ = cmd.Run() // ignore errors — process may not be running
	// Poll until both default KISS ports are free.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !portInUse("127.0.0.1", 8001) && !portInUse("127.0.0.1", 8002) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// portInUse returns true if something is already listening on host:port.
func portInUse(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
