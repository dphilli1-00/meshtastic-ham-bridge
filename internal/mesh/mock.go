package mesh

import (
	"context"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

// MockMode controls how the mock device behaves.
type MockMode int

const (
	// Echo: packets sent via SendPacket loop back to RecvPacket.
	MockEcho MockMode = iota
	// Sink: packets sent via SendPacket are queued; RecvPacket reads them.
	// Use this to observe what the bridge delivers to the mesh side.
	MockSink
	// Source: RecvPacket reads packets pre-loaded via Inject().
	// Use this to simulate incoming mesh traffic.
	MockSource
)

// MockDevice is an in-memory mesh device for testing.
type MockDevice struct {
	mode MockMode
	ch   chan []byte // echo/sink/source channel
}

func NewMock(mode MockMode) *MockDevice {
	return &MockDevice{mode: mode, ch: make(chan []byte, 32)}
}

// Inject pre-loads a packet for Source mode (or injects into Sink from outside).
func (m *MockDevice) Inject(data []byte) {
	m.ch <- data
}

func (m *MockDevice) DeviceType() string { return "mock-mesh" }

func (m *MockDevice) SendText(ctx context.Context, text string) error {
	return m.SendPacket(ctx, []byte(text))
}

func (m *MockDevice) SendPacket(ctx context.Context, data []byte) error {
	switch m.mode {
	case MockEcho, MockSink:
		select {
		case m.ch <- append([]byte(nil), data...):
		case <-ctx.Done():
			return ctx.Err()
		}
	case MockSource:
		// source ignores sends
	}
	return nil
}

func (m *MockDevice) RecvPacket(ctx context.Context) ([]byte, error) {
	select {
	case pkt := <-m.ch:
		return pkt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *MockDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (m *MockDevice) Close() error { return nil }
