// Package discovery finds mesh and ham devices without requiring manual port configuration.
//
// Serial port discovery uses USB VID/PID to identify known Meshtastic/Meshcore hardware.
// Audio device discovery finds Digirigs by name substring.
//
// The "plug in and scan" workflow:
//   before := discovery.SnapshotSerial()
//   // user plugs in device
//   after := discovery.SnapshotSerial()
//   new := discovery.DiffSerial(before, after)
package discovery

import (
	"fmt"
	"strings"

	"go.bug.st/serial/enumerator"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
)

// SerialDevice describes a discovered serial port.
type SerialDevice struct {
	Port        string // e.g. "COM3" or "/dev/ttyUSB0"
	Description string // human-readable name from OS
	VID         string // USB Vendor ID (hex, e.g. "10C4")
	PID         string // USB Product ID (hex, e.g. "EA60")
	Serial      string // USB serial number
	IsUSB       bool
	Kind        DeviceKind
}

// DeviceKind is what we think this device is.
type DeviceKind int

const (
	KindUnknown   DeviceKind = iota
	KindMeshtastic            // likely a Meshtastic node
	KindMeshcore              // likely a Meshcore node
	KindOther                 // USB serial but not recognized
)

func (k DeviceKind) String() string {
	switch k {
	case KindMeshtastic:
		return "meshtastic"
	case KindMeshcore:
		return "meshcore"
	default:
		return "unknown"
	}
}

// Known USB VID:PID combinations for Meshtastic-compatible hardware.
// Meshtastic runs on many boards — we match by common USB-serial chip VID/PIDs.
var knownMeshtastic = []vidpid{
	{"10C4", "EA60"}, // Silicon Labs CP2102 — most common (T-Beam, Heltec, etc.)
	{"1A86", "7523"}, // CH340 — budget boards
	{"1A86", "55D4"}, // CH9102 — newer budget boards
	{"0403", "6001"}, // FTDI FT232 — some RAK boards
	{"0403", "6010"}, // FTDI FT2232
	{"239A", "8029"}, // Adafruit (some mesh boards)
	{"303A", "1001"}, // Espressif USB (ESP32-S3 native USB)
	{"303A", "0002"}, // Espressif USB CDC
}

// Known USB VID:PID for Meshcore hardware.
var knownMeshcore = []vidpid{
	{"303A", "1001"}, // ESP32-S3 (also used by Meshtastic — context-dependent)
	{"10C4", "EA60"}, // CP2102
}

type vidpid struct{ vid, pid string }

// ListSerialDevices returns all USB serial ports with identification.
func ListSerialDevices() ([]SerialDevice, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("enumerate serial ports: %w", err)
	}
	result := make([]SerialDevice, 0, len(ports))
	for _, p := range ports {
		d := SerialDevice{
			Port:        p.Name,
			Description: p.Product,
			VID:         strings.ToUpper(p.VID),
			PID:         strings.ToUpper(p.PID),
			Serial:      p.SerialNumber,
			IsUSB:       p.IsUSB,
			Kind:        KindUnknown,
		}
		if p.IsUSB {
			d.Kind = classify(d.VID, d.PID)
		}
		result = append(result, d)
	}
	return result, nil
}

// FindMeshtastic returns ports that are likely Meshtastic nodes.
func FindMeshtastic() ([]SerialDevice, error) {
	all, err := ListSerialDevices()
	if err != nil {
		return nil, err
	}
	var found []SerialDevice
	for _, d := range all {
		if d.Kind == KindMeshtastic {
			found = append(found, d)
		}
	}
	return found, nil
}

// FindMeshcore returns ports that are likely Meshcore nodes.
func FindMeshcore() ([]SerialDevice, error) {
	all, err := ListSerialDevices()
	if err != nil {
		return nil, err
	}
	var found []SerialDevice
	for _, d := range all {
		if d.Kind == KindMeshcore {
			found = append(found, d)
		}
	}
	return found, nil
}

// SnapshotSerial returns the current set of serial port names.
// Use with DiffSerial to implement plug-in detection.
func SnapshotSerial() (map[string]SerialDevice, error) {
	devices, err := ListSerialDevices()
	if err != nil {
		return nil, err
	}
	m := make(map[string]SerialDevice, len(devices))
	for _, d := range devices {
		m[d.Port] = d
	}
	return m, nil
}

// DiffSerial returns devices present in after but not in before.
// Use this to find a device that was just plugged in.
func DiffSerial(before, after map[string]SerialDevice) []SerialDevice {
	var added []SerialDevice
	for port, d := range after {
		if _, existed := before[port]; !existed {
			added = append(added, d)
		}
	}
	return added
}

// AudioDevice describes a discovered audio device.
type AudioDevice struct {
	Name    string
	IsInput bool
	IsOutput bool
	LikelyDigirig bool
}

// ListAudioDevices returns all audio devices, flagging likely Digirigs.
func ListAudioDevices() ([]AudioDevice, error) {
	names, err := ham.ListAudioDevices()
	if err != nil {
		return nil, err
	}
	result := make([]AudioDevice, 0, len(names))
	for _, name := range names {
		result = append(result, AudioDevice{
			Name:          name,
			LikelyDigirig: isDigirig(name),
		})
	}
	return result, nil
}

// FindDigirigs returns audio devices that are likely Digirigs.
func FindDigirigs() ([]AudioDevice, error) {
	all, err := ListAudioDevices()
	if err != nil {
		return nil, err
	}
	var found []AudioDevice
	for _, d := range all {
		if d.LikelyDigirig {
			found = append(found, d)
		}
	}
	return found, nil
}

func isDigirig(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "digirig") ||
		strings.Contains(name, "cm108") ||
		strings.Contains(name, "usb audio codec") ||
		strings.Contains(name, "usb audio device") // CM108 generic name on Windows
}

func classify(vid, pid string) DeviceKind {
	vp := vidpid{vid, pid}
	for _, known := range knownMeshtastic {
		if vp == known {
			return KindMeshtastic
		}
	}
	// Meshcore shares some VID/PIDs with Meshtastic; default to Meshtastic
	// for common chips since Meshtastic is more prevalent.
	return KindUnknown
}
