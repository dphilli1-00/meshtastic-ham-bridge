package ham

import (
	"context"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

type MockMode int

const (
	MockEcho   MockMode = iota // sent frames loop back to RecvFrame
	MockSink                   // sent frames queued; RecvFrame reads them
	MockSource                 // RecvFrame reads pre-loaded frames via Inject
)

type MockDevice struct {
	mode MockMode
	ch   chan []byte
}

func NewMock(mode MockMode) *MockDevice {
	return &MockDevice{mode: mode, ch: make(chan []byte, 32)}
}

func (m *MockDevice) Inject(data []byte) { m.ch <- data }

func (m *MockDevice) DeviceType() string { return "mock-ham" }

func (m *MockDevice) SendFrame(ctx context.Context, data []byte) error {
	switch m.mode {
	case MockEcho, MockSink:
		select {
		case m.ch <- append([]byte(nil), data...):
		case <-ctx.Done():
			return ctx.Err()
		}
	case MockSource:
		// ignores sends
	}
	return nil
}

func (m *MockDevice) RecvFrame(ctx context.Context) ([]byte, error) {
	select {
	case frame := <-m.ch:
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *MockDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (m *MockDevice) Close() error { return nil }
