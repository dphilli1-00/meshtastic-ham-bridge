package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/bridge"
	"github.com/dphilli/meshtastic-ham-bridge/internal/config"
	"github.com/dphilli/meshtastic-ham-bridge/internal/discovery"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/mesh"
)

func main() {
	var (
		cfgPath     = flag.String("config", "", "path to config.toml (default: platform config dir)")
		genCfg      = flag.Bool("init-config", false, "write a template config and exit")
		listAudio   = flag.Bool("list-audio", false, "list available audio devices and exit")
		discover    = flag.Bool("discover", false, "list all detected serial and audio devices and exit")
		discoverBLE = flag.Bool("discover-ble-all", false, "show ALL BLE devices (debug)")
		testSerial  = flag.String("test-serial", "", "test serial connect to Meshtastic, e.g. COM4")
		testBLE     = flag.String("test-ble", "", "test BLE connect to Meshtastic, e.g. C0:C2:24:70:D8:15")
		verbose     = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if *testSerial != "" {
		testMeshConnect("serial:"+*testSerial, func() (mesh.Device, error) {
			return mesh.ConnectMeshtasticSerial(*testSerial, 115200)
		})
		return
	}

	if *testBLE != "" {
		testMeshConnect("ble:"+*testBLE, func() (mesh.Device, error) {
			return mesh.ConnectMeshtasticBLE(*testBLE)
		})
		return
	}

	if *discoverBLE {
		fmt.Println("=== All BLE Devices (10s scan) ===")
		all, err := discovery.ScanBLERaw(10 * time.Second)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		} else if len(all) == 0 {
			fmt.Println("  (none found — is Bluetooth on?)")
		} else {
			for _, d := range all {
				fmt.Printf("  %-35s  %s  RSSI:%d\n", d.Name, d.Address, d.RSSI)
			}
		}
		return
	}

	if *discover {
		fmt.Println("=== Serial Devices ===")
		serials, err := discovery.ListSerialDevices()
		if err != nil {
			slog.Error("serial discovery failed", "err", err)
		} else if len(serials) == 0 {
			fmt.Println("  (none found)")
		} else {
			for _, d := range serials {
				fmt.Printf("  %-8s  VID:%s PID:%s  %-12s  %s\n",
					d.Port, d.VID, d.PID, d.Kind, d.Description)
			}
		}

		fmt.Println("\n=== BLE Devices (5s scan) ===")
		bles, err := discovery.ScanBLERaw(5 * time.Second)
		if err != nil {
			slog.Error("BLE discovery failed", "err", err)
		} else if len(bles) == 0 {
			fmt.Println("  (none found)")
		} else {
			for _, d := range bles {
				name := d.Name
				hint := ""
				if name == "" {
					// Already-paired devices don't advertise name on Windows.
					// Show MAC suffix as a hint — matches Meshtastic node name format.
					hint = fmt.Sprintf("  (paired? suffix=%s)", discovery.MeshtasticMACToName(d.Address))
				}
				kind := ""
				if d.Kind != discovery.KindUnknown {
					kind = "  " + d.Kind.String()
				}
				fmt.Printf("  %-30s  %s  RSSI:%-4d%s%s\n",
					name, d.Address, d.RSSI, kind, hint)
			}
		}

		fmt.Println("\n=== Audio Devices ===")
		audios, err := discovery.ListAudioDevices()
		if err != nil {
			slog.Error("audio discovery failed", "err", err)
		} else if len(audios) == 0 {
			fmt.Println("  (none found)")
		} else {
			for _, d := range audios {
				tag := ""
				if d.LikelyDigirig {
					tag = "  ← Digirig"
				}
				fmt.Printf("  %s%s\n", d.Name, tag)
			}
		}
		return
	}

	if *listAudio {
		devices, err := ham.ListAudioDevices()
		if err != nil {
			slog.Error("failed to list audio devices", "err", err)
			os.Exit(1)
		}
		for _, d := range devices {
			fmt.Println(d)
		}
		return
	}

	if *genCfg {
		path := config.DefaultPath()
		if *cfgPath != "" {
			path = *cfgPath
		}
		if err := config.Template().SaveTo(path); err != nil {
			slog.Error("failed to write config", "err", err)
			os.Exit(1)
		}
		fmt.Printf("config written to %s\n", path)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	if len(cfg.Bridge) == 0 {
		slog.Warn("no bridges configured — run with -init-config to create a template")
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	for _, bcfg := range cfg.Bridge {
		meshDev, err := buildMesh(bcfg.Mesh)
		if err != nil {
			slog.Error("failed to connect mesh device", "bridge", bcfg.Name, "err", err)
			os.Exit(1)
		}
		hamDev, err := buildHam(bcfg.Ham)
		if err != nil {
			slog.Error("failed to connect ham device", "bridge", bcfg.Name, "err", err)
			os.Exit(1)
		}
		b := bridge.New(bcfg.Name, meshDev, hamDev)
		go b.Run(ctx)
		slog.Info("bridge started", "name", bcfg.Name)
	}

	<-ctx.Done()
	slog.Info("shutting down")
}

func buildMesh(cfg config.MeshConfig) (mesh.Device, error) {
	switch cfg.Type {
	case "meshtastic":
		// BLE address takes priority over serial
		if cfg.BLEAddress != "" {
			slog.Info("connecting Meshtastic via BLE", "addr", cfg.BLEAddress)
			return mesh.ConnectMeshtasticBLE(cfg.BLEAddress)
		}
		port := cfg.Port
		if port == "" {
			found, err := discovery.FindMeshtastic()
			if err != nil {
				return nil, fmt.Errorf("meshtastic discovery: %w", err)
			}
			if len(found) == 0 {
				return nil, fmt.Errorf("no Meshtastic device found — plug one in or set port/ble_address in config")
			}
			if len(found) > 1 {
				slog.Warn("multiple Meshtastic devices found, using first",
					"port", found[0].Port, "count", len(found))
			}
			port = found[0].Port
			slog.Info("discovered Meshtastic device", "port", port)
		}
		return mesh.ConnectMeshtasticSerial(port, cfg.BaudRate)
	case "meshcore":
		port := cfg.Port
		if port == "" {
			found, err := discovery.FindMeshcore()
			if err != nil {
				return nil, fmt.Errorf("meshcore discovery: %w", err)
			}
			if len(found) == 0 {
				return nil, fmt.Errorf("no Meshcore device found — plug one in or set port in config")
			}
			port = found[0].Port
			slog.Info("discovered Meshcore device", "port", port)
		}
		return mesh.ConnectMeshcoreSerial(port, cfg.BaudRate)
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
	case "audio+rigctl":
		// Audio modem for data, rigctld for PTT/CAT control (e.g. IC-705)
		audio, err := ham.NewAudio(cfg.AudioDevice)
		if err != nil {
			return nil, fmt.Errorf("audio: %w", err)
		}
		return ham.NewRigctl(audio, cfg.RigctlHost, cfg.RigctlPort)
	default:
		return nil, fmt.Errorf("unknown ham type %q", cfg.Type)
	}
}

// testMeshConnect connects to a mesh device, reads packets for 10s, and prints them.
func testMeshConnect(label string, connect func() (mesh.Device, error)) {
	fmt.Printf("Connecting to %s...\n", label)
	dev, err := connect()
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		return
	}
	defer dev.Close()
	fmt.Printf("OK — connected as %s\n", dev.DeviceType())
	// Send WantConfigId to trigger the node to stream its config over FromRadio.
	// ToRadio.want_config_id = field 3, varint: tag=0x18, value=42 (0x2a)
	wantConfig := []byte{0x18, 0x2a}
	if err := dev.SendPacket(context.Background(), wantConfig); err != nil {
		fmt.Printf("send WantConfigId failed: %v\n", err)
	} else {
		fmt.Println("Sent WantConfigId — waiting for FromRadio packets...")
	}
	fmt.Println("Reading packets for 10s (Ctrl-C to stop early)...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count := 0
	for {
		pkt, err := dev.RecvPacket(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break // timeout — normal exit
			}
			fmt.Printf("recv error: %v\n", err)
			break
		}
		count++
		fmt.Printf("  pkt #%d  %d bytes: % X\n", count, len(pkt), pkt)
	}
	fmt.Printf("Done — received %d packet(s)\n", count)
}
