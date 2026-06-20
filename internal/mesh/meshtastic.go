package mesh

import (
	"context"
	"fmt"
	"io"

	"go.bug.st/serial"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// MeshtasticDevice connects to a Meshtastic node over serial.
// TODO: implement BLE transport (task #17)
// TODO: implement protobuf send/recv (task #25)
type MeshtasticDevice struct {
	port serial.Port
	recv chan []byte
	done chan struct{}
}

// ConnectSerial opens a serial connection to a Meshtastic node.
func ConnectMeshtasticSerial(portName string, baud int) (*MeshtasticDevice, error) {
	if baud == 0 {
		baud = 115200
	}
	mode := &serial.Mode{BaudRate: baud}
	p, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("meshtastic serial open %s: %w", portName, err)
	}
	d := &MeshtasticDevice{
		port: p,
		recv: make(chan []byte, 32),
		done: make(chan struct{}),
	}
	go d.readLoop()
	return d, nil
}

func (d *MeshtasticDevice) readLoop() {
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

func (d *MeshtasticDevice) DeviceType() string { return "meshtastic-serial" }

func (d *MeshtasticDevice) SendText(ctx context.Context, text string) error {
	// TODO: wrap in Meshtastic protobuf ToRadio message (task #25)
	return d.SendPacket(ctx, []byte(text))
}

func (d *MeshtasticDevice) SendPacket(_ context.Context, data []byte) error {
	// TODO: wrap in Meshtastic protobuf ToRadio message (task #25)
	_, err := d.port.Write(data)
	return err
}

func (d *MeshtasticDevice) RecvPacket(ctx context.Context) ([]byte, error) {
	select {
	case pkt, ok := <-d.recv:
		if !ok {
			return nil, fmt.Errorf("meshtastic: connection closed")
		}
		return pkt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *MeshtasticDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (d *MeshtasticDevice) Close() error {
	close(d.done)
	return d.port.Close()
}
