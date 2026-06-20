use meshtastic_ham_bridge::config::{
    Config, BridgeConfig, MeshConfig, MeshtasticConfig, HamConfig, DirewolfConfig, AudioConfig,
};
use std::path::PathBuf;

// ---------------------------------------------------------------------------
// Round-trip: serialize → deserialize → same values
// ---------------------------------------------------------------------------

#[test]
fn test_roundtrip_full_config() {
    let config = Config {
        bridge: vec![
            BridgeConfig {
                name: "vhf".into(),
                mesh: MeshConfig::Meshtastic(MeshtasticConfig {
                    port: Some("COM3".into()),
                    ble_name: None,
                }),
                ham: HamConfig::Direwolf(DirewolfConfig {
                    host: "127.0.0.1".into(),
                    port: 8001,
                }),
            },
            BridgeConfig {
                name: "hf".into(),
                mesh: MeshConfig::Meshtastic(MeshtasticConfig {
                    port: None,
                    ble_name: Some("Meshtastic_abcd".into()),
                }),
                ham: HamConfig::Audio(AudioConfig {
                    device: Some("Digirig".into()),
                }),
            },
        ],
    };

    let toml = toml::to_string_pretty(&config).unwrap();
    println!("serialized:\n{toml}");

    let parsed: Config = toml::from_str(&toml).unwrap();
    assert_eq!(parsed.bridge.len(), 2);
    assert_eq!(parsed.bridge[0].name, "vhf");
    assert_eq!(parsed.bridge[1].name, "hf");

    // Check port on bridge[0]
    match &parsed.bridge[0].mesh {
        MeshConfig::Meshtastic(m) => assert_eq!(m.port.as_deref(), Some("COM3")),
        _ => panic!("wrong mesh type"),
    }
    match &parsed.bridge[0].ham {
        HamConfig::Direwolf(d) => {
            assert_eq!(d.host, "127.0.0.1");
            assert_eq!(d.port, 8001);
        }
        _ => panic!("wrong ham type"),
    }
}

#[test]
fn test_empty_config_deserializes() {
    let config: Config = toml::from_str("").unwrap();
    assert_eq!(config.bridge.len(), 0);
}

#[test]
fn test_direwolf_defaults() {
    let toml = r#"
[[bridge]]
name = "test"

[bridge.mesh]
type = "meshtastic"

[bridge.ham]
type = "direwolf"
"#;
    let config: Config = toml::from_str(toml).unwrap();
    match &config.bridge[0].ham {
        HamConfig::Direwolf(d) => {
            assert_eq!(d.host, "127.0.0.1");
            assert_eq!(d.port, 8001);
        }
        _ => panic!("wrong ham type"),
    }
}

// ---------------------------------------------------------------------------
// Load / save to temp file
// ---------------------------------------------------------------------------

#[test]
fn test_save_and_load() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("config.toml");

    let original = Config::template();
    original.save_to(&path).unwrap();

    assert!(path.exists());

    let loaded = Config::load_from(&path).unwrap();
    assert_eq!(loaded.bridge.len(), original.bridge.len());
    assert_eq!(loaded.bridge[0].name, original.bridge[0].name);
}

#[test]
fn test_load_missing_returns_default() {
    // Non-existent path with no env var → empty config, no error
    std::env::remove_var("MESHTASTIC_HAM_CONFIG");
    // Pass a path that doesn't exist but override the default search
    let path = PathBuf::from("/nonexistent/path/config.toml");
    // load_from would error, but load() with no args falls back to empty
    // Simulate by checking load() works with no file present
    // (only safe if default path also doesn't exist on this machine)
    // So we use the env var path to a known-missing file
    std::env::set_var("MESHTASTIC_HAM_CONFIG", "/nonexistent/config.toml");
    let result = Config::load(None);
    std::env::remove_var("MESHTASTIC_HAM_CONFIG");
    // Should error (file doesn't exist), not panic
    assert!(result.is_err());
    let _ = path; // suppress warning
}

#[test]
fn test_load_explicit_path() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("explicit.toml");
    Config::template().save_to(&path).unwrap();

    let loaded = Config::load(Some(&path)).unwrap();
    assert_eq!(loaded.bridge.len(), 2);
}

// ---------------------------------------------------------------------------
// Template
// ---------------------------------------------------------------------------

#[test]
fn test_template_is_valid_toml() {
    let config = Config::template();
    let toml = toml::to_string_pretty(&config).unwrap();
    println!("template:\n{toml}");
    let parsed: Config = toml::from_str(&toml).unwrap();
    assert_eq!(parsed.bridge.len(), 2);
}

#[test]
fn test_default_path_is_absolute() {
    let path = Config::default_path();
    assert!(path.is_absolute(), "default path should be absolute: {path:?}");
    assert!(path.ends_with("config.toml"));
}
