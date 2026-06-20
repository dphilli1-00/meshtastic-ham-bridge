package ham_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
)

// fakeRigctld simulates a rigctld server for testing.
func startFakeRigctld(t *testing.T) (port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			cmd := strings.TrimSpace(scanner.Text())
			switch {
			case cmd == "_":
				// handshake: return rig name then RPRT 0
				conn.Write([]byte("hamlib-rigctld\nRPRT 0\n"))
			case strings.HasPrefix(cmd, "T "):
				conn.Write([]byte("RPRT 0\n"))
			case cmd == "f":
				// get frequency: value line then RPRT 0
				conn.Write([]byte("14225000\nRPRT 0\n"))
			default:
				conn.Write([]byte("RPRT 0\n"))
			}
		}
		close(done)
	}()
	return port, func() { ln.Close(); <-done }
}

func TestRigctlPTT(t *testing.T) {
	port, stop := startFakeRigctld(t)
	defer stop()

	inner := ham.NewMock(ham.MockEcho)
	rig, err := ham.NewRigctl(inner, "127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	defer rig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// SendFrame should key PTT, send, unkey PTT — all without error
	if err := rig.SendFrame(ctx, []byte("test frame")); err != nil {
		t.Fatal(err)
	}

	// Frame should have gone through the inner mock and be readable
	frame, err := rig.RecvFrame(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(frame) != "test frame" {
		t.Fatalf("got %q", frame)
	}
}

func TestRigctlFrequency(t *testing.T) {
	port, stop := startFakeRigctld(t)
	defer stop()

	inner := ham.NewMock(ham.MockSink)
	rig, err := ham.NewRigctl(inner, "127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	defer rig.Close()

	hz, err := rig.GetFrequency()
	if err != nil {
		t.Fatal(err)
	}
	if hz != 14225000 {
		t.Fatalf("got %d Hz", hz)
	}
}

// TestRigctlHardware connects to a real rigctld — skipped if not running.
// Start rigctld first: rigctld -m 3085 -r COM3 -s 19200  (IC-705)
func TestRigctlHardware(t *testing.T) {
	inner := ham.NewMock(ham.MockSink)
	rig, err := ham.NewRigctl(inner, "127.0.0.1", 4532)
	if err != nil {
		t.Skipf("rigctld not available (start it to run this test): %v", err)
	}
	defer rig.Close()

	hz, err := rig.GetFrequency()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rig frequency: %d Hz", hz)

	// PTT on then off
	if err := rig.PTT(true); err != nil {
		t.Fatal("PTT on:", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := rig.PTT(false); err != nil {
		t.Fatal("PTT off:", err)
	}
}
