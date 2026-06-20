package mesh

import (
	"context"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// Device is anything that can send and receive mesh radio packets.
type Device interface {
	// DeviceType returns a human-readable identifier e.g. "meshtastic-serial".
	DeviceType() string

	// SendText sends a plain text message over the mesh.
	SendText(ctx context.Context, text string) error

	// SendPacket sends raw bytes as a mesh packet.
	SendPacket(ctx context.Context, data []byte) error

	// RecvPacket blocks until a packet arrives or ctx is cancelled.
	RecvPacket(ctx context.Context) ([]byte, error)

	// Status returns the current device status.
	Status(ctx context.Context) (types.DeviceStatus, error)

	// Close disconnects from the device.
	Close() error
}
