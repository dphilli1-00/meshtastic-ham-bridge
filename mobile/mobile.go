// Package mobile exposes the bridge to iOS and Android via gomobile bind.
//
// Build iOS framework:
//   go install golang.org/x/mobile/cmd/gomobile@latest
//   gomobile init
//   gomobile bind -target ios -o MeshtasticHamBridge.xcframework github.com/dphilli/meshtastic-ham-bridge/mobile
//
// Build Android AAR:
//   gomobile bind -target android -o meshtastic-ham-bridge.aar github.com/dphilli/meshtastic-ham-bridge/mobile
//
// Gomobile restrictions: exported types may only use primitives, string, []byte, error,
// and other bound types. No channels, maps, or generics in exported signatures.
package mobile

import (
	"context"
	"fmt"
	"strings"

	"github.com/dphilli/meshtastic-ham-bridge/internal/bridge"
	"github.com/dphilli/meshtastic-ham-bridge/internal/config"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/mesh"
)

// Bridge is the gomobile-exported handle to a running bridge instance.
type Bridge struct {
	cancel  context.CancelFunc
	bridges []*bridge.Bridge
}

// Start starts all bridges defined in the config file.
// configPath: path to config.toml, or "" to use the platform default.
func Start(configPath string) (*Bridge, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Bridge) == 0 {
		return nil, fmt.Errorf("no bridges configured")
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{cancel: cancel}

	for _, bcfg := range cfg.Bridge {
		meshDev, err := buildMesh(bcfg.Mesh)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("mesh device %q: %w", bcfg.Name, err)
		}
		hamDev, err := buildHam(bcfg.Ham)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("ham device %q: %w", bcfg.Name, err)
		}
		br := bridge.New(bcfg.Name, meshDev, hamDev)
		b.bridges = append(b.bridges, br)
		go br.Run(ctx)
	}
	return b, nil
}

// StartAudio starts a single bridge with the given mesh serial port and audio device.
// audioDevice: substring match for audio device name, or "" for system default.
// meshPort: serial port e.g. "COM3" or "/dev/ttyUSB0", or "" to use config.
func StartAudio(meshPort string, audioDevice string) (*Bridge, error) {
	var meshDev mesh.Device
	var err error

	if meshPort != "" {
		meshDev, err = mesh.ConnectMeshtasticSerial(meshPort, 115200)
	} else {
		return nil, fmt.Errorf("meshPort required (discovery not yet implemented)")
	}
	if err != nil {
		return nil, fmt.Errorf("mesh connect: %w", err)
	}

	hamDev, err := ham.NewAudio(audioDevice)
	if err != nil {
		return nil, fmt.Errorf("audio device: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	br := bridge.New("audio-bridge", meshDev, hamDev)
	b := &Bridge{cancel: cancel, bridges: []*bridge.Bridge{br}}
	go br.Run(ctx)
	return b, nil
}

// Stop stops all running bridges.
func (b *Bridge) Stop() {
	b.cancel()
}

// ListAudioDevices returns a newline-separated list of available audio device names.
func ListAudioDevices() (string, error) {
	devices, err := ham.ListAudioDevices()
	if err != nil {
		return "", err
	}
	return strings.Join(devices, "\n"), nil
}

func buildMesh(cfg config.MeshConfig) (mesh.Device, error) {
	switch cfg.Type {
	case "meshtastic":
		if cfg.Port == "" {
			return nil, fmt.Errorf("port required")
		}
		return mesh.ConnectMeshtasticSerial(cfg.Port, cfg.BaudRate)
	case "meshcore":
		if cfg.Port == "" {
			return nil, fmt.Errorf("port required")
		}
		return mesh.ConnectMeshcoreSerial(cfg.Port, cfg.BaudRate)
	default:
		return nil, fmt.Errorf("unknown mesh type %q", cfg.Type)
	}
}

func buildHam(cfg config.HamConfig) (ham.Device, error) {
	switch cfg.Type {
	case "direwolf":
		return ham.ConnectDirewolf(cfg.Host, cfg.Port)
	case "audio":
		return ham.NewAudio(cfg.AudioDevice)
	default:
		return nil, fmt.Errorf("unknown ham type %q", cfg.Type)
	}
}
