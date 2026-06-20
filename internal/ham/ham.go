package ham

import (
	"context"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// Device is anything that can send and receive ham radio frames.
type Device interface {
	// DeviceType returns a human-readable identifier e.g. "direwolf-kiss".
	DeviceType() string

	// SendFrame sends a raw AX.25 frame.
	SendFrame(ctx context.Context, data []byte) error

	// RecvFrame blocks until a frame arrives or ctx is cancelled.
	RecvFrame(ctx context.Context) ([]byte, error)

	// Status returns the current device status.
	Status(ctx context.Context) (types.DeviceStatus, error)

	// Close disconnects from the device.
	Close() error
}
