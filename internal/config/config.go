package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Config is the top-level config structure.
type Config struct {
	Bridge []BridgeConfig `toml:"bridge"`
}

type BridgeConfig struct {
	Name string     `toml:"name"`
	Mesh MeshConfig `toml:"mesh"`
	Ham  HamConfig  `toml:"ham"`
}

// MeshConfig — type field selects which sub-config is used.
type MeshConfig struct {
	Type       string `toml:"type"`        // "meshtastic" or "meshcore"
	Port       string `toml:"port"`        // serial port; empty = auto-discover
	BLEAddress string `toml:"ble_address"` // BLE MAC e.g. "C0:C2:24:70:D8:15"; takes priority over serial
	BLEName    string `toml:"ble_name"`    // BLE device name substring (informational)
	BaudRate   int    `toml:"baud_rate"`   // default 115200
}

// HamConfig — type field selects which sub-config is used.
type HamConfig struct {
	Type         string `toml:"type"`          // "direwolf", "audio", "audio+rigctl", "ardop"
	Host         string `toml:"host"`          // direwolf/ardop/rigctld host
	Port         int    `toml:"port"`          // direwolf/ardop/rigctld port
	AudioDevice  string `toml:"audio_device"`  // substring match; empty = default
	DirewolfPath string `toml:"direwolf_path"` // path to direwolf binary; empty = search PATH + common locations
	DirewolfConf string `toml:"direwolf_conf"` // path to direwolf.conf; empty = prompt interactively if TTY
	// rigctld — used when type is "audio+rigctl"
	RigctlHost string `toml:"rigctl_host"` // default 127.0.0.1
	RigctlPort int    `toml:"rigctl_port"` // default 4532
}

// Load reads config using the standard search order:
//  1. explicit path argument
//  2. $MESHTASTIC_HAM_CONFIG env var
//  3. platform default path
//  4. empty config (discovery will fill in devices)
func Load(explicitPath string) (*Config, error) {
	// Track whether the path came from an explicit source (arg or env var).
	// If explicit and missing → error. If default and missing → empty config.
	path := explicitPath
	explicit := path != ""
	if path == "" {
		path = os.Getenv("MESHTASTIC_HAM_CONFIG")
		if path != "" {
			explicit = true
		}
	}
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if explicit {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		// Default path missing — return empty config (discovery fills in devices)
		return &Config{}, nil
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	// Apply defaults
	for i := range cfg.Bridge {
		b := &cfg.Bridge[i]
		if b.Ham.Type == "direwolf" && b.Ham.Host == "" {
			b.Ham.Host = "127.0.0.1"
		}
		if b.Ham.Type == "direwolf" && b.Ham.Port == 0 {
			b.Ham.Port = 8001
		}
		if b.Ham.Type == "ardop" && b.Ham.Host == "" {
			b.Ham.Host = "127.0.0.1"
		}
		if b.Ham.Type == "ardop" && b.Ham.Port == 0 {
			b.Ham.Port = 8515
		}
		if b.Mesh.BaudRate == 0 {
			b.Mesh.BaudRate = 115200
		}
	}
	return &cfg, nil
}

// Save writes the config to the default path.
func (c *Config) Save() error {
	return c.SaveTo(DefaultPath())
}

// SaveTo writes the config to an explicit path.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// DefaultPath returns the platform config file path.
func DefaultPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "meshtastic-ham-bridge", "config.toml")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "meshtastic-ham-bridge", "config.toml")
	default: // linux, pi
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "meshtastic-ham-bridge", "config.toml")
	}
}

// Template returns a starter config with sensible defaults and no ports set.
func Template() *Config {
	return &Config{
		Bridge: []BridgeConfig{
			{
				Name: "vhf-local",
				Mesh: MeshConfig{Type: "meshtastic", BaudRate: 115200},
				Ham:  HamConfig{Type: "direwolf", Host: "127.0.0.1", Port: 8001},
			},
			{
				Name: "hf-backbone",
				Mesh: MeshConfig{Type: "meshtastic", BaudRate: 115200},
				Ham:  HamConfig{Type: "audio", AudioDevice: "Digirig"},
			},
		},
	}
}
