package ham

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/platform"
)

// DirewolfAudioDevice is an audio device as reported by Direwolf itself.
type DirewolfAudioDevice struct {
	Index int
	Name  string
}

// ListDirewolfAudioDevices runs Direwolf with a dummy config and parses its
// audio device list from stderr. This gives us the exact names Direwolf
// expects in its ADEVICE config line.
func ListDirewolfAudioDevices() (inputs []DirewolfAudioDevice, outputs []DirewolfAudioDevice, err error) {
	p := platform.Detect()
	if p.DirewolfPath == "" {
		return nil, nil, fmt.Errorf("direwolf not found — set DIREWOLF_PATH or install direwolf")
	}

	// Use NUL (Windows) or /dev/null (Unix) as config — Direwolf prints the
	// full device list before failing to find any configuration.
	nullDev := "NUL"
	if p.OS != "windows" {
		nullDev = "/dev/null"
	}

	// Run from Direwolf's own directory so it finds its data files (tocalls.yaml, etc.)
	direwolfDir := p.DirewolfPath[:strings.LastIndexAny(p.DirewolfPath, `/\`)]
	cmd := exec.Command(p.DirewolfPath, "-t", "0", "-c", nullDev)
	cmd.Dir = direwolfDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	// Direwolf will eventually block on audio init — kill it after it prints the device list.
	_ = cmd.Start()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}

	inputs, outputs = parseDirewolfDeviceList(buf.String())
	if len(inputs) == 0 && len(outputs) == 0 {
		return nil, nil, fmt.Errorf("could not parse device list from direwolf output (len=%d):\n---\n%s\n---", buf.Len(), buf.String())
	}
	return inputs, outputs, nil
}

// parseDirewolfDeviceList extracts device lists from Direwolf's output.
// Direwolf prints blocks like:
//
//	Available audio input devices for receive (*=selected):
//	    0: Device Name
//	    1: Another Device
//	Available audio output devices for transmit (*=selected):
//	    0: Device Name
func parseDirewolfDeviceList(output string) (inputs, outputs []DirewolfAudioDevice) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var current *[]DirewolfAudioDevice
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if strings.Contains(lower, "available audio input") {
			current = &inputs
			continue
		}
		if strings.Contains(lower, "available audio output") {
			current = &outputs
			continue
		}
		if current == nil {
			continue
		}
		// Lines look like: "    0: Device Name" or " *  0: Device Name"
		trimmed := strings.TrimLeft(line, " *")
		var idx int
		var name string
		if n, _ := fmt.Sscanf(trimmed, "%d:", &idx); n == 1 {
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx >= 0 {
				name = strings.TrimSpace(trimmed[colonIdx+1:])
				*current = append(*current, DirewolfAudioDevice{Index: idx, Name: name})
			}
		} else if strings.TrimSpace(line) == "" {
			// blank line ends the block
			current = nil
		}
	}
	return
}
