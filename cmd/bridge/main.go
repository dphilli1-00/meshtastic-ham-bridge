package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dphilli/meshtastic-ham-bridge/internal/bridge"
	"github.com/dphilli/meshtastic-ham-bridge/internal/config"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/mesh"
)

func main() {
	var (
		cfgPath  = flag.String("config", "", "path to config.toml (default: platform config dir)")
		genCfg   = flag.Bool("init-config", false, "write a template config and exit")
		listAudio = flag.Bool("list-audio", false, "list available audio devices and exit")
		verbose  = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

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
		if cfg.Port == "" {
			return nil, fmt.Errorf("meshtastic: port required (discovery not yet implemented)")
		}
		return mesh.ConnectMeshtasticSerial(cfg.Port, cfg.BaudRate)
	case "meshcore":
		if cfg.Port == "" {
			return nil, fmt.Errorf("meshcore: port required (discovery not yet implemented)")
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
