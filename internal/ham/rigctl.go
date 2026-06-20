package ham

// RigctlDevice wraps another HamDevice (typically audio) and adds CAT/PTT
// control via rigctld (hamlib's network daemon).
//
// rigctld handles: PTT, frequency, mode — works with any hamlib-supported rig.
// IC-705, Kenwood, Yaesu, etc. all supported via the same interface.
//
// Start rigctld before running the bridge:
//   rigctld -m 3085 -r /dev/ttyUSB0 -s 19200   # IC-705 via USB
//   rigctld -m 2 -r /dev/ttyUSB0               # any rig in list mode
//
// rigctld protocol is simple newline-terminated text over TCP (default port 4532).

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// RigctlDevice wraps an inner HamDevice and keys PTT via rigctld before transmitting.
type RigctlDevice struct {
	inner  Device
	conn   net.Conn
	reader *bufio.Reader
	host   string
	port   int
}

// NewRigctl connects to rigctld and wraps the given inner device.
// inner is typically an AudioDevice (AFSK modem) or DirewolfDevice.
// host/port: rigctld address, default 127.0.0.1:4532.
func NewRigctl(inner Device, host string, port int) (*RigctlDevice, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 4532
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("rigctld connect %s:%d: %w", host, port, err)
	}
	r := &RigctlDevice{
		inner:  inner,
		conn:   conn,
		reader: bufio.NewReader(conn),
		host:   host,
		port:   port,
	}
	// Verify connection with a ping
	if _, err := r.command("_"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("rigctld handshake failed: %w", err)
	}
	return r, nil
}

func (r *RigctlDevice) DeviceType() string {
	return fmt.Sprintf("rigctl(%s)", r.inner.DeviceType())
}

// SendFrame keys PTT, sends the frame via the inner device, then unkeys.
func (r *RigctlDevice) SendFrame(ctx context.Context, data []byte) error {
	if err := r.setPTT(true); err != nil {
		return fmt.Errorf("ptt on: %w", err)
	}
	err := r.inner.SendFrame(ctx, data)
	// Always unkey PTT even if send failed
	if pttErr := r.setPTT(false); pttErr != nil {
		// Log but don't mask the original error
		fmt.Printf("rigctl: ptt off failed: %v\n", pttErr)
	}
	return err
}

// RecvFrame delegates to the inner device.
func (r *RigctlDevice) RecvFrame(ctx context.Context) ([]byte, error) {
	return r.inner.RecvFrame(ctx)
}

func (r *RigctlDevice) Status(ctx context.Context) (types.DeviceStatus, error) {
	return r.inner.Status(ctx)
}

func (r *RigctlDevice) Close() error {
	r.setPTT(false) // safety: ensure PTT is off
	r.conn.Close()
	return r.inner.Close()
}

// SetFrequency sets the rig frequency in Hz.
func (r *RigctlDevice) SetFrequency(hz uint64) error {
	_, err := r.command(fmt.Sprintf("F %d", hz))
	return err
}

// SetMode sets the rig mode e.g. "FM", "USB", "LSB", "PKT-FM".
func (r *RigctlDevice) SetMode(mode string) error {
	_, err := r.command(fmt.Sprintf("M %s 0", mode))
	return err
}

// GetFrequency returns the current rig frequency in Hz.
func (r *RigctlDevice) GetFrequency() (uint64, error) {
	resp, err := r.command("f")
	if err != nil {
		return 0, err
	}
	var hz uint64
	fmt.Sscanf(strings.TrimSpace(resp), "%d", &hz)
	return hz, nil
}

// PTT directly — useful for testing.
func (r *RigctlDevice) PTT(on bool) error {
	return r.setPTT(on)
}

func (r *RigctlDevice) setPTT(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	_, err := r.command(fmt.Sprintf("T %s", v))
	return err
}

// command sends a rigctld command and returns the response.
// rigctld protocol:
//   set commands → "RPRT 0\n"
//   get commands → "value\nRPRT 0\n"
func (r *RigctlDevice) command(cmd string) (string, error) {
	r.conn.SetDeadline(time.Now().Add(2 * time.Second))
	defer r.conn.SetDeadline(time.Time{})

	if _, err := fmt.Fprintf(r.conn, "%s\n", cmd); err != nil {
		return "", err
	}

	var data string
	for {
		line, err := r.reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "RPRT ") {
			var code int
			fmt.Sscanf(line, "RPRT %d", &code)
			if code != 0 {
				return "", fmt.Errorf("rigctld error %d for command %q", code, cmd)
			}
			return strings.TrimSpace(data), nil
		}
		// Data line — accumulate (most commands return only one data line)
		if data == "" {
			data = line
		} else {
			data += "\n" + line
		}
	}
}
