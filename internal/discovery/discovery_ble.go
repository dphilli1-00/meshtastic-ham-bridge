package discovery

// BLE discovery via tinygo.org/x/bluetooth.
//   - Windows: pure-Go WinRT bindings (no CGo, Windows 10+ required)
//   - Linux: BlueZ via D-Bus
//   - macOS: CoreBluetooth (CGo)
//
// Meshtastic BLE service UUID: 6ba1b218-15a8-461f-9fa8-5d6646720a11
// Meshtastic nodes advertise their node name in the local name advertisement.
// Meshcore: match by name prefix "Meshcore".

import (
	"fmt"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

// BLEDevice describes a discovered BLE device.
type BLEDevice struct {
	Name    string
	Address string // MAC address (Linux) or UUID (macOS)
	RSSI    int16
	Kind    DeviceKind
}

// Meshtastic BLE service UUID: 6ba1b218-15a8-461f-9fa8-5d6646720a11
// NewUUID takes bytes in big-endian (RFC 4122) order.
var meshtasticServiceUUID = bluetooth.NewUUID([16]byte{
	0x6b, 0xa1, 0xb2, 0x18,
	0x15, 0xa8,
	0x46, 0x1f,
	0x9f, 0xa8,
	0x5d, 0xca, 0xe2, 0x73, 0xea, 0xfd,
})

// looksLikeMeshtastic matches short Meshtastic node names like "slvr_d815", "bl45_4562".
// Format is exactly <1-4 alphanum chars>_<4 hex digits> — nothing more, nothing less.
// Govee/other devices use longer prefixes like "Govee_H6022_1A69" which won't match.
func looksLikeMeshtastic(name string) bool {
	idx := strings.Index(name, "_")
	if idx < 1 || idx > 4 {
		return false // prefix must be 1-4 chars
	}
	if len(name) != idx+1+4 {
		return false // must be exactly prefix + "_" + 4 chars, no more
	}
	suffix := name[idx+1:]
	for _, c := range suffix {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ScanBLE scans for BLE devices for the given duration and returns all found.
// If duration is 0, scans for 5 seconds.
func ScanBLE(duration time.Duration) ([]BLEDevice, error) {
	if duration == 0 {
		duration = 5 * time.Second
	}

	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("BLE enable: %w", err)
	}

	var found []BLEDevice
	seen := map[string]bool{}

	done := make(chan struct{})
	go func() {
		time.Sleep(duration)
		close(done)
	}()

	err := adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		select {
		case <-done:
			a.StopScan()
			return
		default:
		}

		addr := result.Address.String()
		if seen[addr] {
			return
		}
		seen[addr] = true

		name := result.LocalName()
		kind := classifyBLE(name, result)
		if kind == KindUnknown {
			return // only report recognized devices
		}

		found = append(found, BLEDevice{
			Name:    name,
			Address: addr,
			RSSI:    result.RSSI,
			Kind:    kind,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("BLE scan: %w", err)
	}

	return found, nil
}

// ScanBLERaw returns every BLE device found, regardless of kind. Useful for debugging.
func ScanBLERaw(duration time.Duration) ([]BLEDevice, error) {
	if duration == 0 {
		duration = 5 * time.Second
	}
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("BLE enable: %w", err)
	}
	var found []BLEDevice
	seen := map[string]bool{}
	done := make(chan struct{})
	go func() {
		time.Sleep(duration)
		close(done)
	}()
	err := adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		select {
		case <-done:
			a.StopScan()
			return
		default:
		}
		addr := result.Address.String()
		if seen[addr] {
			return
		}
		seen[addr] = true
		found = append(found, BLEDevice{
			Name:    result.LocalName(),
			Address: addr,
			RSSI:    result.RSSI,
			Kind:    classifyBLE(result.LocalName(), result),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("BLE scan: %w", err)
	}
	return found, nil
}

// FindMeshtasticBLE scans and returns Meshtastic nodes.
func FindMeshtasticBLE(duration time.Duration) ([]BLEDevice, error) {
	all, err := ScanBLE(duration)
	if err != nil {
		return nil, err
	}
	var found []BLEDevice
	for _, d := range all {
		if d.Kind == KindMeshtastic {
			found = append(found, d)
		}
	}
	return found, nil
}

func classifyBLE(name string, result bluetooth.ScanResult) DeviceKind {
	// Service UUID is the most reliable signal
	if result.HasServiceUUID(meshtasticServiceUUID) {
		return KindMeshtastic
	}
	// Name-based fallback: explicit prefix or short-name pattern (e.g. "slvr_d815")
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "MESHTASTIC") || looksLikeMeshtastic(name) {
		return KindMeshtastic
	}
	if strings.HasPrefix(upper, "MESHCORE") {
		return KindMeshcore
	}
	// Nameless device: Windows doesn't return local names for already-paired devices.
	// Meshtastic node name suffix = last 2 bytes of MAC (e.g. D8:15 → "d815").
	// We can't confirm it's Meshtastic from MAC alone, so mark as unknown.
	// The --discover-ble-all output will show the MAC so the user can configure by address.
	return KindUnknown
}

// MeshtasticMACToName guesses a Meshtastic node's short name suffix from its MAC.
// e.g. "C0:C2:24:70:D8:15" → "d815"
func MeshtasticMACToName(addr string) string {
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(parts[len(parts)-2] + parts[len(parts)-1])
}
