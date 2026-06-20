package bridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/bridge"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/mesh"
)

func TestMeshEcho(t *testing.T) {
	meshDev := mesh.NewMock(mesh.MockEcho)
	hamDev := ham.NewMock(ham.MockSink)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := bridge.New("test", meshDev, hamDev)
	go b.Run(ctx)

	if err := meshDev.SendText(ctx, "hello"); err != nil {
		t.Fatal(err)
	}

	recv, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel2()

	pkt, err := meshDev.RecvPacket(recv)
	if err != nil {
		t.Fatalf("timed out waiting for echo: %v", err)
	}
	if string(pkt) != "hello" {
		t.Fatalf("got %q, want %q", pkt, "hello")
	}
}

func TestBridgeFullPath(t *testing.T) {
	// mesh(Sink) → bridge → ham(Echo) → bridge → mesh(Sink)
	meshDev := mesh.NewMock(mesh.MockSink)
	hamDev := ham.NewMock(ham.MockEcho)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := bridge.New("test", meshDev, hamDev)
	go b.Run(ctx)

	time.Sleep(10 * time.Millisecond)

	if err := meshDev.SendText(ctx, "bridge-test"); err != nil {
		t.Fatal(err)
	}

	recv, cancel2 := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel2()

	pkt, err := meshDev.RecvPacket(recv)
	if err != nil {
		t.Fatalf("timed out waiting for packet through bridge: %v", err)
	}
	if string(pkt) != "bridge-test" {
		t.Fatalf("got %q, want %q", pkt, "bridge-test")
	}
}

func TestMeshInject(t *testing.T) {
	meshDev := mesh.NewMock(mesh.MockSink)

	meshDev.Inject([]byte("injected"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	pkt, err := meshDev.RecvPacket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(pkt) != "injected" {
		t.Fatalf("got %q", pkt)
	}
}

func TestSourceMode(t *testing.T) {
	meshDev := mesh.NewMock(mesh.MockSource)
	meshDev.Inject([]byte("from-mesh"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	pkt, err := meshDev.RecvPacket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(pkt) != "from-mesh" {
		t.Fatalf("got %q", pkt)
	}
}
