package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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
		cfgPath        = flag.String("config", "", "path to config.toml (default: platform config dir)")
		genCfg         = flag.Bool("init-config", false, "write a template config and exit")
		listAudio      = flag.Bool("list-audio", false, "list available audio devices and exit")
		discover       = flag.Bool("discover", false, "list all detected serial and audio devices and exit")
		discoverBLE    = flag.Bool("discover-ble-all", false, "show ALL BLE devices (debug)")
		testSerial     = flag.String("test-serial", "", "test serial connect to Meshtastic, e.g. COM4")
		testBLE        = flag.String("test-ble", "", "test BLE connect to Meshtastic, e.g. C0:C2:24:70:D8:15")
		setupDirewolf  = flag.String("setup-direwolf", "", "run interactive Direwolf config wizard, write to given path (e.g. direwolf-tx.conf)")
		testDirewolf   = flag.String("test-direwolf", "", "RF loopback test: --test-direwolf tx.conf,rx.conf")
		testDirewolfRX = flag.Bool("rx", false, "with single --test-direwolf conf: listen only, print received frames")
		testAudio      = flag.String("test-audio", "", "audio modem loopback test: --test-audio \"tx device\",\"rx device\"")
		testAudioTones = flag.String("test-audio-tones", "", "raw tone test (no HDLC): --test-audio-tones \"tx device\",\"rx device\"")
		pttPath        = flag.String("ptt", "", "CM108 HID path for PTT (leave empty for VOX/auto-detect)")
		listCM108      = flag.Bool("list-cm108", false, "list CM108 HID devices and exit")
		verbose        = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if *listCM108 {
		ham.ListCM108()
		return
	}

	if *setupDirewolf != "" {
		kissPort := 8001
		if strings.Contains(*setupDirewolf, "rx") {
			kissPort = 8002
		}
		if err := ham.RunDirewolfSetup(*setupDirewolf, kissPort); err != nil {
			fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *testDirewolf != "" {
		parts := strings.SplitN(*testDirewolf, ",", 2)

		payload := []byte("direwolf-loopback-test")
		frame := []byte{
			0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x00, // dst: APRS
			0xA8, 0x8A, 0x9C, 0xA8, 0x40, 0x40, 0x02, // src: TEST-1
			0x03, 0xF0, // control + PID
		}
		frame = append(frame, payload...)

		ham.KillAllDirewolf()

		// Single config: TX loop or RX listen.
		if len(parts) == 1 {
			conf := strings.TrimSpace(parts[0])
			kissPort, err := ham.ReadKISSPort(conf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "reading config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Launching Direwolf with %s...\n", conf)
			dev, err := ham.LaunchDirewolf(conf, kissPort)
			if err != nil {
				fmt.Fprintf(os.Stderr, "direwolf failed: %v\n", err)
				os.Exit(1)
			}
			defer dev.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			go func() { <-sig; cancel() }()

			if *testDirewolfRX {
				fmt.Println("Listening for frames. Ctrl-C to stop.")
				for {
					f, err := dev.RecvFrame(ctx)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						fmt.Fprintf(os.Stderr, "recv: %v\n", err)
						return
					}
					fmt.Printf("RX (%d bytes): % X\n", len(f), f)
					if len(f) > 16 {
						fmt.Printf("  info: %q\n", f[16:])
					}
				}
			}

			fmt.Println("Sending frame every second. Ctrl-C to stop.")
			for {
				if err := dev.SendFrame(ctx, frame); err != nil {
					fmt.Fprintf(os.Stderr, "send: %v\n", err)
					break
				}
				fmt.Printf("TX: % X\n", frame)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
			}
			return
		}

		// Two configs: loopback assert.
		// Always launch the lower-port config first. Direwolf 1.8.1 always
		// attempts to bind 8001 as a default; if the higher-port instance
		// launches first it steals 8001 and the lower-port instance can't start.
		confA, confB := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		portA, err := ham.ReadKISSPort(confA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading config: %v\n", err)
			os.Exit(1)
		}
		portB, err := ham.ReadKISSPort(confB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading config: %v\n", err)
			os.Exit(1)
		}
		if portA == portB {
			fmt.Fprintf(os.Stderr, "both configs use port %d — they must use different KISSPORT values\n", portA)
			os.Exit(1)
		}

		txConf, txPort, rxConf, rxPort := confA, portA, confB, portB
		if portA > portB {
			txConf, txPort, rxConf, rxPort = confB, portB, confA, portA
		}

		fmt.Println("Launching TX Direwolf...")
		tx, err := ham.LaunchDirewolf(txConf, txPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TX direwolf failed: %v\n", err)
			os.Exit(1)
		}
		defer tx.Close()

		fmt.Println("Launching RX Direwolf...")
		rx, err := ham.LaunchDirewolf(rxConf, rxPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "RX direwolf failed: %v\n", err)
			os.Exit(1)
		}
		defer rx.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() { <-sig; cancel() }()

		// Print received frames as they arrive.
		go func() {
			for {
				f, err := rx.RecvFrame(ctx)
				if err != nil {
					return
				}
				if bytes.Contains(f, payload) {
					fmt.Printf("PASS — RX matched (%d bytes): % X\n", len(f), f)
				} else {
					fmt.Printf("RX (%d bytes, no match): % X\n", len(f), f)
				}
			}
		}()

		fmt.Println("Sending frame every second. Ctrl-C to stop.")
		for {
			if err := tx.SendFrame(ctx, frame); err != nil {
				fmt.Fprintf(os.Stderr, "send: %v\n", err)
				break
			}
			fmt.Printf("TX: % X\n", frame)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}
		return
	}

	if *testAudio != "" {
		parts := strings.SplitN(*testAudio, ",", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "usage: --test-audio \"tx device name\",\"rx device name\"\n")
			os.Exit(1)
		}
		txName := strings.Trim(strings.TrimSpace(parts[0]), "\"")
		rxName := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() { <-sig; cancel() }()

		runAudioTest(ctx, txName, rxName, *pttPath)
		return
	}

	if *testAudioTones != "" {
		parts := strings.SplitN(*testAudioTones, ",", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "usage: --test-audio-tones \"tx device\",\"rx device\"\n")
			os.Exit(1)
		}
		txName := strings.Trim(strings.TrimSpace(parts[0]), "\"")
		rxName := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		fmt.Printf("Opening TX audio device %q...\n", txName)
		tx, err := ham.NewAudioTX(txName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TX audio: %v\n", err)
			os.Exit(1)
		}
		defer tx.Close()

		fmt.Printf("Opening RX audio device %q...\n", rxName)
		rx, err := ham.NewAudioRX(rxName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "RX audio: %v\n", err)
			os.Exit(1)
		}
		defer rx.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() { <-sig; cancel() }()

		// Print raw bit decisions as M/S characters, plus disc value every 80 bits.
		go func() {
			var line []byte
			for {
				select {
				case b := <-rx.RawBits():
					if b {
						line = append(line, 'M')
					} else {
						line = append(line, 'S')
					}
					if len(line) >= 80 {
						fmt.Printf("bits: %s  energy=%.3f disc=%.6f\n", line, rx.Energy(), rx.Disc())
						line = line[:0]
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		// Send 1s mark then 1s space, repeatedly.
		// 1200 baud × 1s = 1200 bits per tone.
		nBits := 1200
		pattern := make([]bool, nBits*2)
		for i := range pattern {
			pattern[i] = i < nBits // first half = mark (1200 Hz), second = space (2200 Hz)
		}
		fmt.Println("Sending 1s×mark(1200Hz) + 1s×space(2200Hz) every 3s. Expect: MMM...SSS...")
		fmt.Println("energy=0 means RX device not receiving signal.")
		for {
			fmt.Println("TX: tones")
			tx.SendTones(pattern)
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}

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
				break
			}
			fmt.Printf("recv error: %v\n", err)
			break
		}
		count++
		fmt.Printf("  pkt #%d  %d bytes: % X\n", count, len(pkt), pkt)
	}
	fmt.Printf("Done — received %d packet(s)\n", count)
}
