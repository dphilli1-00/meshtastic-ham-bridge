/// Configuration system — TOML file, multiple bridge instances.
///
/// File location (in priority order):
///   1. Path passed explicitly (CLI --config flag)
///   2. $MESHTASTIC_HAM_CONFIG env var
///   3. Platform default:
///        Windows:  %APPDATA%\meshtastic-ham-bridge\config.toml
///        macOS:    ~/Library/Application Support/meshtastic-ham-bridge/config.toml
///        Linux:    ~/.config/meshtastic-ham-bridge/config.toml
///
/// Example config.toml:
///
///   [[bridge]]
///   name = "vhf-local"
///
///   [bridge.mesh]
///   type = "meshtastic"
///   port = "COM3"        # omit to use discovery
///
///   [bridge.ham]
///   type = "direwolf"
///   host = "127.0.0.1"
///   port = 8001
///
///   [[bridge]]
///   name = "hf-backbone"
///
///   [bridge.mesh]
///   type = "meshcore"
///   port = "COM4"
///
///   [bridge.ham]
///   type = "audio"
///   device = "Digirig"   # substring match; omit for system default

use std::path::{Path, PathBuf};
use serde::{Deserialize, Serialize};
use crate::error::{Error, Result};

// ---------------------------------------------------------------------------
// Top-level config
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Config {
    #[serde(default)]
    pub bridge: Vec<BridgeConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BridgeConfig {
    /// Human-readable name shown in logs and UI.
    pub name: String,
    pub mesh: MeshConfig,
    pub ham: HamConfig,
}

// ---------------------------------------------------------------------------
// Mesh device config
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum MeshConfig {
    Meshtastic(MeshtasticConfig),
    Meshcore(MeshcoreConfig),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MeshtasticConfig {
    /// Serial port, e.g. "COM3" or "/dev/ttyUSB0".
    /// None = use discovery.
    pub port: Option<String>,
    /// BLE device name substring, e.g. "Meshtastic_1234".
    /// Used if port is None and BLE is preferred.
    pub ble_name: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MeshcoreConfig {
    pub port: Option<String>,
    pub ble_name: Option<String>,
}

// ---------------------------------------------------------------------------
// Ham device config
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum HamConfig {
    Direwolf(DirewolfConfig),
    Audio(AudioConfig),
    Ardop(ArdopConfig),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DirewolfConfig {
    #[serde(default = "default_direwolf_host")]
    pub host: String,
    #[serde(default = "default_direwolf_port")]
    pub port: u16,
}

fn default_direwolf_host() -> String { "127.0.0.1".into() }
fn default_direwolf_port() -> u16 { 8001 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AudioConfig {
    /// Substring match against system audio device names.
    /// None = system default.
    pub device: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArdopConfig {
    #[serde(default = "default_ardop_host")]
    pub host: String,
    #[serde(default = "default_ardop_port")]
    pub port: u16,
}

fn default_ardop_host() -> String { "127.0.0.1".into() }
fn default_ardop_port() -> u16 { 8515 }

// ---------------------------------------------------------------------------
// Load / save
// ---------------------------------------------------------------------------

impl Config {
    /// Load from an explicit path.
    pub fn load_from(path: &Path) -> Result<Self> {
        let text = std::fs::read_to_string(path)
            .map_err(|e| Error::Unknown(format!("cannot read config {}: {e}", path.display())))?;
        toml::from_str(&text)
            .map_err(|e| Error::Unknown(format!("invalid config: {e}")))
    }

    /// Load using the standard search order.
    pub fn load(explicit_path: Option<&Path>) -> Result<Self> {
        if let Some(p) = explicit_path {
            return Self::load_from(p);
        }
        if let Ok(p) = std::env::var("MESHTASTIC_HAM_CONFIG") {
            return Self::load_from(Path::new(&p));
        }
        let default = Self::default_path();
        if default.exists() {
            Self::load_from(&default)
        } else {
            // No config file — return empty config (discovery fills in devices)
            Ok(Config::default())
        }
    }

    /// Save to the default location, creating directories as needed.
    pub fn save(&self) -> Result<()> {
        let path = Self::default_path();
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| Error::Unknown(e.to_string()))?;
        }
        let text = toml::to_string_pretty(self)
            .map_err(|e| Error::Unknown(e.to_string()))?;
        std::fs::write(&path, text)
            .map_err(|e| Error::Unknown(e.to_string()))
    }

    /// Save to an explicit path.
    pub fn save_to(&self, path: &Path) -> Result<()> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| Error::Unknown(e.to_string()))?;
        }
        let text = toml::to_string_pretty(self)
            .map_err(|e| Error::Unknown(e.to_string()))?;
        std::fs::write(path, text)
            .map_err(|e| Error::Unknown(e.to_string()))
    }

    /// Platform-appropriate default config path.
    pub fn default_path() -> PathBuf {
        dirs::config_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join("meshtastic-ham-bridge")
            .join("config.toml")
    }

    /// Generate a template config with sensible defaults.
    /// Ports are None — user fills them in or lets discovery handle it.
    pub fn template() -> Self {
        Config {
            bridge: vec![
                BridgeConfig {
                    name: "vhf-local".into(),
                    mesh: MeshConfig::Meshtastic(MeshtasticConfig {
                        port: None,
                        ble_name: None,
                    }),
                    ham: HamConfig::Direwolf(DirewolfConfig {
                        host: "127.0.0.1".into(),
                        port: 8001,
                    }),
                },
                BridgeConfig {
                    name: "hf-backbone".into(),
                    mesh: MeshConfig::Meshtastic(MeshtasticConfig {
                        port: None,
                        ble_name: None,
                    }),
                    ham: HamConfig::Audio(AudioConfig {
                        device: Some("Digirig".into()),
                    }),
                },
            ],
        }
    }
}
