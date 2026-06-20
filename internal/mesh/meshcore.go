package mesh

import (
	"context"
	"fmt"
	"io"

	"go.bug.st/serial"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// MeshcoreDevice connects to a Meshcore node over serial.
// TODO: BLE transport (task #17)
// TODO: Meshcore protocol framing (task #25)
type MeshcoreDevice struct {
	port serial.Port
	recv chan []byte
	done chan struct{}
}

func ConnectMeshcoreSerial(portName string, baud int) (*MeshcoreDevice, error) {
	if baud == 0 {
		baud = 115200
	}
	p, err := serial.Open(portName, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, fmt.Errorf("meshcore serial open %s: %w", portName, err)
	}
	d := &MeshcoreDevice{
		port: p,
		recv: make(chan []byte, 32),
		done: make(chan struct{}),
	}
	go d.readLoop()
	return d, nil
}

func (d *MeshcoreDevice) readLoop() {
	defer close(d.recv)
	buf := make([]byte, 4096)
	for {
		select {
		case <-d.done:
			return
		default:
		}
		n, err := d.port.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			continue
		}
		if n > 0 {
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			select {
			case d.recv <- pkt:
			case <-d.done:
				return
			}
		}
	}
}

func (d *MeshcoreDevice) DeviceType() string { return "meshcore-serial" }

func (d *MeshcoreDevice) SendText(ctx context.Context, text string) error {
	return d.SendPacket(ctx, []byte(text))
}

func (d *MeshcoreDevice) SendPacket(_ context.Context, data []byte) error {
	_, err := d.port.Write(data)
	return err
}

func (d *MeshcoreDevice) RecvPacket(ctx context.Context) ([]byte, error) {
	select {
	case pkt, ok := <-d.recv:
		if !ok {
			return nil, fmt.Errorf("meshcore: connection closed")
		}
		return pkt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *MeshcoreDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (d *MeshcoreDevice) Close() error {
	close(d.done)
	return d.port.Close()
}
