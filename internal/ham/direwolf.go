package ham

import (
	"context"
	"fmt"
	"net"
	"os/exec"

	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

const (
	kissFlag   = 0xC0
	kissEscape = 0xDB
	kissEscFlag = 0xDC
	kissEscEsc  = 0xDD
)

// DirewolfDevice connects to Direwolf via KISS over TCP.
// If cmd is non-nil, it was launched by LaunchDirewolf and will be killed on Close.
type DirewolfDevice struct {
	conn net.Conn
	recv chan []byte
	done chan struct{}
	cmd  *exec.Cmd // non-nil if we spawned the process
}

func ConnectDirewolf(host string, port int) (*DirewolfDevice, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("direwolf connect %s:%d: %w", host, port, err)
	}
	d := &DirewolfDevice{
		conn: conn,
		recv: make(chan []byte, 32),
		done: make(chan struct{}),
	}
	go d.readLoop()
	return d, nil
}

func (d *DirewolfDevice) readLoop() {
	defer close(d.recv)
	buf := make([]byte, 1)
	for {
		// Wait for opening 0xC0
		for {
			_, err := d.conn.Read(buf)
			if err != nil {
				return
			}
			if buf[0] == kissFlag {
				break
			}
		}
		// Skip command byte
		if _, err := d.conn.Read(buf); err != nil {
			return
		}
		// Read payload until closing 0xC0
		var frame []byte
		for {
			_, err := d.conn.Read(buf)
			if err != nil {
				return
			}
			b := buf[0]
			if b == kissFlag {
				break
			}
			if b == kissEscape {
				if _, err := d.conn.Read(buf); err != nil {
					return
				}
				switch buf[0] {
				case kissEscFlag:
					frame = append(frame, kissFlag)
				case kissEscEsc:
					frame = append(frame, kissEscape)
				}
				continue
			}
			frame = append(frame, b)
		}
		if len(frame) > 0 {
			select {
			case d.recv <- frame:
			case <-d.done:
				return
			}
		}
	}
}

func kissEncode(data []byte) []byte {
	frame := []byte{kissFlag, 0x00} // flag + data frame command
	for _, b := range data {
		switch b {
		case kissFlag:
			frame = append(frame, kissEscape, kissEscFlag)
		case kissEscape:
			frame = append(frame, kissEscape, kissEscEsc)
		default:
			frame = append(frame, b)
		}
	}
	return append(frame, kissFlag)
}

func (d *DirewolfDevice) DeviceType() string { return "direwolf-kiss" }

func (d *DirewolfDevice) SendFrame(_ context.Context, data []byte) error {
	_, err := d.conn.Write(kissEncode(data))
	return err
}

func (d *DirewolfDevice) RecvFrame(ctx context.Context) ([]byte, error) {
	select {
	case frame, ok := <-d.recv:
		if !ok {
			return nil, fmt.Errorf("direwolf: connection closed")
		}
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *DirewolfDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (d *DirewolfDevice) Close() error {
	close(d.done)
	err := d.conn.Close()
	if d.cmd != nil {
		d.cmd.Process.Kill()
		d.cmd.Wait()
	}
	return err
}
