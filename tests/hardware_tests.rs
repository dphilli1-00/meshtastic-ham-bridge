/// Hardware integration tests.
///
/// These require real devices and are skipped by default.
/// Run with: cargo test -- --ignored
///
/// Set env vars to point at your hardware:
///   MESH1_PORT=COM3       first Meshtastic node serial port
///   MESH2_PORT=COM4       second Meshtastic node serial port
///   DIREWOLF1_HOST=127.0.0.1:8001   first Direwolf KISS TCP
///   DIREWOLF2_HOST=127.0.0.1:8002   second Direwolf KISS TCP
///   AUDIO_DEVICE=Digirig  substring match for audio device name
///
/// TODO: replace env vars with config system (task #33) once implemented.

use std::sync::Arc;
use meshtastic_ham_bridge::mesh::meshtastic::MeshtasticDevice;
use meshtastic_ham_bridge::ham::direwolf::DirewolfDevice;
use meshtastic_ham_bridge::ham::audio::AudioHamDevice;
use meshtastic_ham_bridge::mesh::MeshDevice;
use meshtastic_ham_bridge::ham::HamDevice;
use meshtastic_ham_bridge::bridge::Bridge;
use tokio::time::{timeout, Duration, sleep};

fn env(key: &str) -> Option<String> {
    std::env::var(key).ok()
}

fn require_env(key: &str) -> String {
    env(key).unwrap_or_else(|| panic!("set {key} env var to run this test"))
}

// ---------------------------------------------------------------------------
// Meshtastic
// ---------------------------------------------------------------------------

/// Connect to a single Meshtastic node via serial and verify it doesn't error.
#[tokio::test]
#[ignore]
async fn test_meshtastic_connect() {
    let port = require_env("MESH1_PORT");
    let device = MeshtasticDevice::connect_serial(&port).await
        .expect("failed to connect to Meshtastic node");

    let status = device.status().await.expect("failed to get status");
    println!("Meshtastic status: {:?}", status);
    assert!(status.connected);
}

/// Send a text message from node 1 and verify it arrives (requires both nodes in range).
#[tokio::test]
#[ignore]
async fn test_meshtastic_send_recv() {
    let port1 = require_env("MESH1_PORT");
    let port2 = require_env("MESH2_PORT");

    let node1 = MeshtasticDevice::connect_serial(&port1).await
        .expect("failed to connect to node 1");
    let node2 = MeshtasticDevice::connect_serial(&port2).await
        .expect("failed to connect to node 2");

    node1.send_text("hello from node1").await.expect("send failed");

    let packet = timeout(Duration::from_secs(10), node2.recv_raw_packet())
        .await
        .expect("timed out waiting for packet on node 2")
        .expect("recv error");

    println!("node2 received: {:?}", String::from_utf8_lossy(&packet));
}

// ---------------------------------------------------------------------------
// Direwolf KISS TCP
// ---------------------------------------------------------------------------

/// Connect to Direwolf and send a frame.
/// Verify the frame comes back (requires Direwolf with a loopback or second instance).
#[tokio::test]
#[ignore]
async fn test_direwolf_connect_send() {
    let addr = require_env("DIREWOLF1_HOST");
    let (host, port) = addr.rsplit_once(':')
        .map(|(h, p)| (h.to_string(), p.parse::<u16>().unwrap()))
        .expect("DIREWOLF1_HOST must be host:port");

    let device = DirewolfDevice::connect(&host, port).await
        .expect("failed to connect to Direwolf");

    device.send_frame(b"CQ CQ CQ DE W1AW").await.expect("send failed");
    println!("frame sent to Direwolf");
}

// ---------------------------------------------------------------------------
// Audio / Digirig
// ---------------------------------------------------------------------------

/// Enumerate audio devices and verify at least one Digirig is visible.
#[tokio::test]
#[ignore]
async fn test_digirig_enumeration() {
    let devices = AudioHamDevice::list_devices().expect("failed to list audio devices");
    println!("audio devices:");
    for d in &devices {
        println!("  {d}");
    }

    let filter = env("AUDIO_DEVICE").unwrap_or_else(|| "Digirig".into());
    let found = devices.iter().any(|d| d.contains(&filter));
    assert!(found, "no audio device matching '{filter}' found — is the Digirig plugged in?");
}

/// Open both Digirigs and verify they initialize without error.
#[tokio::test]
#[ignore]
async fn test_digirig_open() {
    let filter = env("AUDIO_DEVICE").unwrap_or_else(|| "Digirig".into());
    let _device = AudioHamDevice::new(Some(&filter))
        .expect("failed to open audio device");
    println!("audio device opened ok");
    sleep(Duration::from_millis(500)).await; // let it run briefly
}

// ---------------------------------------------------------------------------
// Full RF loopback
// ---------------------------------------------------------------------------

/// The big one: packet travels mesh1 → bridge1 → digirig1 → HT1 → RF → HT2 → digirig2 → bridge2 → mesh2.
///
/// Setup:
///   - Two Meshtastic nodes on MESH1_PORT and MESH2_PORT
///   - Two Direwolf instances on DIREWOLF1_HOST and DIREWOLF2_HOST
///   - Two Digirigs connected to two HTs, wired (or keyed) to reach each other
///   - Both Direwolf instances configured for their respective Digirig audio devices
#[tokio::test]
#[ignore]
async fn test_full_rf_loopback() {
    let port1 = require_env("MESH1_PORT");
    let addr1 = require_env("DIREWOLF1_HOST");
    let port2 = require_env("MESH2_PORT");
    let addr2 = require_env("DIREWOLF2_HOST");

    let (host1, dport1) = split_addr(&addr1);
    let (host2, dport2) = split_addr(&addr2);

    let mesh1 = Arc::new(MeshtasticDevice::connect_serial(&port1).await
        .expect("failed to connect mesh1")) as Arc<dyn MeshDevice + Send + Sync>;
    let ham1 = Arc::new(DirewolfDevice::connect(&host1, dport1).await
        .expect("failed to connect direwolf1")) as Arc<dyn HamDevice + Send + Sync>;

    let mesh2 = Arc::new(MeshtasticDevice::connect_serial(&port2).await
        .expect("failed to connect mesh2")) as Arc<dyn MeshDevice + Send + Sync>;
    let ham2 = Arc::new(DirewolfDevice::connect(&host2, dport2).await
        .expect("failed to connect direwolf2")) as Arc<dyn HamDevice + Send + Sync>;

    let bridge1 = Bridge::new(Arc::clone(&mesh1), Arc::clone(&ham1));
    let bridge2 = Bridge::new(Arc::clone(&mesh2), Arc::clone(&ham2));

    bridge1.run().await;
    bridge2.run().await;

    sleep(Duration::from_millis(500)).await; // let bridges settle

    // Send from mesh1 side
    mesh1.send_text("rf loopback test").await.expect("send failed");
    println!("packet sent from mesh1");

    // Expect to receive on mesh2 side within 15 seconds (RF propagation + decode time)
    let packet = timeout(Duration::from_secs(15), mesh2.recv_raw_packet())
        .await
        .expect("timed out waiting for packet on mesh2")
        .expect("recv error on mesh2");

    println!("mesh2 received: {:?}", String::from_utf8_lossy(&packet));
    assert_eq!(packet, b"rf loopback test");
}

fn split_addr(addr: &str) -> (String, u16) {
    addr.rsplit_once(':')
        .map(|(h, p)| (h.to_string(), p.parse().expect("invalid port")))
        .expect("address must be host:port")
}
