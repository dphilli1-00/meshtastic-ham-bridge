package discovery_test

import (
	"testing"

	"github.com/dphilli/meshtastic-ham-bridge/internal/discovery"
)

func TestSnapshotDiff(t *testing.T) {
	before := map[string]discovery.SerialDevice{
		"COM3": {Port: "COM3", Kind: discovery.KindMeshtastic},
		"COM4": {Port: "COM4", Kind: discovery.KindUnknown},
	}
	after := map[string]discovery.SerialDevice{
		"COM3": {Port: "COM3", Kind: discovery.KindMeshtastic},
		"COM4": {Port: "COM4", Kind: discovery.KindUnknown},
		"COM5": {Port: "COM5", Kind: discovery.KindMeshtastic}, // newly plugged in
	}

	added := discovery.DiffSerial(before, after)
	if len(added) != 1 {
		t.Fatalf("expected 1 new device, got %d", len(added))
	}
	if added[0].Port != "COM5" {
		t.Fatalf("expected COM5, got %s", added[0].Port)
	}
}

func TestDiffNoChange(t *testing.T) {
	snap := map[string]discovery.SerialDevice{
		"COM3": {Port: "COM3"},
	}
	added := discovery.DiffSerial(snap, snap)
	if len(added) != 0 {
		t.Fatalf("expected no new devices, got %d", len(added))
	}
}

// TestListSerialDevices enumerates real ports — passes even with no devices connected.
func TestListSerialDevices(t *testing.T) {
	devices, err := discovery.ListSerialDevices()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found %d serial port(s):", len(devices))
	for _, d := range devices {
		t.Logf("  %s  VID:%s PID:%s  kind:%s  %s",
			d.Port, d.VID, d.PID, d.Kind, d.Description)
	}
}

// TestListAudioDevices enumerates real audio devices.
func TestListAudioDevices(t *testing.T) {
	devices, err := discovery.ListAudioDevices()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found %d audio device(s):", len(devices))
	for _, d := range devices {
		digirig := ""
		if d.LikelyDigirig {
			digirig = " ← likely Digirig"
		}
		t.Logf("  %s%s", d.Name, digirig)
	}
}
