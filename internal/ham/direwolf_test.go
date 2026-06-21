package ham_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
)

// TestDirewolfSetup runs the interactive config wizard to generate direwolf-tx.conf
// and direwolf-rx.conf. Run this once manually before TestDirewolfRFLoopback.
//
//	go test ./internal/ham/ -run TestDirewolfSetup -v
func TestDirewolfSetup(t *testing.T) {
	for _, name := range []string{"direwolf-tx.conf", "direwolf-rx.conf"} {
		if _, err := os.Stat(name); err == nil {
			t.Logf("%s already exists, skipping", name)
			continue
		}
		t.Logf("generating %s...", name)
		if err := ham.RunDirewolfSetup(name, 8001); err != nil {
			t.Fatalf("setup %s: %v", name, err)
		}
	}
}

// TestDirewolfRFLoopback is a hardware integration test.
//
// Requirements:
//   - Two radios on the same frequency, squelch open or below signal level
//   - Two Digirigs (or audio interfaces), one per radio
//   - Direwolf installed and on PATH
//   - direwolf-tx.conf and direwolf-rx.conf in the working directory
//     (run `go test -run TestDirewolfRFLoopback` once interactively to generate them)
//
// Run with:
//
//	go test ./internal/ham/ -run TestDirewolfRFLoopback -v -timeout 30s
//
// Skipped automatically if direwolf is not on PATH or configs are missing.
func TestDirewolfRFLoopback(t *testing.T) {
	tx, err := ham.LaunchDirewolf("direwolf-tx.conf", 8001)
	if err != nil {
		t.Skipf("TX Direwolf not available: %v", err)
	}
	defer tx.Close()

	rx, err := ham.LaunchDirewolf("direwolf-rx.conf", 8002)
	if err != nil {
		t.Skipf("RX Direwolf not available: %v", err)
	}
	defer rx.Close()

	// AX.25 UI frame: src=W1AW-1, dst=APRS, payload="loopback test"
	// This is a minimal valid AX.25 frame for testing — Direwolf will decode it.
	testFrame := buildAX25UIFrame("W1AW-1", "APRS", []byte("loopback test"))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Start listening before transmitting.
	type result struct {
		frame []byte
		err   error
	}
	got := make(chan result, 1)
	go func() {
		f, err := rx.RecvFrame(ctx)
		got <- result{f, err}
	}()

	// Small delay to ensure RX is listening before TX fires.
	time.Sleep(200 * time.Millisecond)

	if err := tx.SendFrame(ctx, testFrame); err != nil {
		t.Fatalf("send frame: %v", err)
	}
	t.Log("frame transmitted, waiting for RX...")

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("recv frame: %v", r.err)
		}
		// AX.25 frames may have control/PID bytes stripped differently by Direwolf
		// depending on version — compare the info field (payload) only.
		if !bytes.Contains(r.frame, []byte("loopback test")) {
			t.Fatalf("payload not found in received frame\nsent:     %x\nreceived: %x", testFrame, r.frame)
		}
		t.Logf("RF loopback OK — received %d bytes", len(r.frame))

	case <-ctx.Done():
		t.Fatal("timeout waiting for RF loopback — check: squelch level, radio frequency, Digirig audio routing, Direwolf config")
	}
}

// buildAX25UIFrame constructs a minimal AX.25 UI frame.
// Format: dst(7) + src(7) + control(1) + pid(1) + info
// Addresses are left-shifted one bit per AX.25 spec; last byte of src has end-of-address bit set.
func buildAX25UIFrame(src, dst string, info []byte) []byte {
	encAddr := func(call string, last bool) []byte {
		// Parse callsign and SSID
		ssid := byte(0)
		if len(call) > 0 {
			for i, c := range call {
				if c == '-' {
					var n int
					for _, d := range call[i+1:] {
						n = n*10 + int(d-'0')
					}
					ssid = byte(n & 0x0F)
					call = call[:i]
					break
				}
			}
		}
		b := make([]byte, 7)
		for i := 0; i < 6; i++ {
			if i < len(call) {
				b[i] = call[i] << 1
			} else {
				b[i] = ' ' << 1
			}
		}
		b[6] = (ssid << 1) & 0x1E
		if last {
			b[6] |= 0x01 // end-of-address bit
		}
		return b
	}

	frame := encAddr(dst, false)
	frame = append(frame, encAddr(src, true)...)
	frame = append(frame, 0x03) // control: UI frame
	frame = append(frame, 0xF0) // PID: no layer 3
	frame = append(frame, info...)
	return frame
}
