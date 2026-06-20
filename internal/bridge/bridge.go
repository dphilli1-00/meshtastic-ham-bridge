package bridge

import (
	"context"
	"log/slog"

	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/mesh"
)

// Bridge moves packets between a mesh device and a ham device in both directions.
type Bridge struct {
	name string
	mesh mesh.Device
	ham  ham.Device
}

func New(name string, m mesh.Device, h ham.Device) *Bridge {
	return &Bridge{name: name, mesh: m, ham: h}
}

// Run starts both bridge loops and blocks until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) {
	slog.Info("bridge starting", "name", b.name,
		"mesh", b.mesh.DeviceType(), "ham", b.ham.DeviceType())

	go b.meshToHam(ctx)
	go b.hamToMesh(ctx)

	<-ctx.Done()
	slog.Info("bridge stopping", "name", b.name)
}

func (b *Bridge) meshToHam(ctx context.Context) {
	for {
		pkt, err := b.mesh.RecvPacket(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("mesh recv error", "name", b.name, "err", err)
			continue
		}
		if err := b.ham.SendFrame(ctx, pkt); err != nil {
			slog.Warn("ham send error", "name", b.name, "err", err)
		}
	}
}

func (b *Bridge) hamToMesh(ctx context.Context) {
	for {
		frame, err := b.ham.RecvFrame(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("ham recv error", "name", b.name, "err", err)
			continue
		}
		if err := b.mesh.SendPacket(ctx, frame); err != nil {
			slog.Warn("mesh send error", "name", b.name, "err", err)
		}
	}
}
