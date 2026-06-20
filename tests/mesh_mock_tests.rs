use std::sync::Arc;
use meshtastic_ham_bridge::mesh::mock::{MockMeshDevice, MockMode};
use meshtastic_ham_bridge::mesh::MeshDevice;
use tokio::time::{timeout, Duration};

// Option 1: inject_packet pushes into Sink device, recv_raw_packet pulls it back out.
// Simulates the bridge delivering a ham→mesh packet to the mesh device.
#[tokio::test]
async fn test_inject_packet_sink() {
    let mesh = MockMeshDevice::new(MockMode::Sink);

    mesh.inject_packet(b"injected".to_vec()).await;

    let packet = timeout(Duration::from_millis(100), mesh.recv_raw_packet())
        .await
        .expect("timed out")
        .unwrap();

    assert_eq!(packet, b"injected");
}

// inject multiple packets, verify ordering (channel is FIFO)
#[tokio::test]
async fn test_inject_packet_ordering() {
    let mesh = MockMeshDevice::new(MockMode::Sink);

    mesh.inject_packet(b"first".to_vec()).await;
    mesh.inject_packet(b"second".to_vec()).await;
    mesh.inject_packet(b"third".to_vec()).await;

    for expected in [b"first".as_ref(), b"second", b"third"] {
        let packet = timeout(Duration::from_millis(100), mesh.recv_raw_packet())
            .await
            .expect("timed out")
            .unwrap();
        assert_eq!(packet, expected);
    }
}

// Option 2: Source mode — test pre-loads packets via inject_packet(),
// then reads them out via recv_raw_packet() as if the mesh device is generating traffic.
// send_packet() is a no-op in Source mode.
#[tokio::test]
async fn test_source_mode_recv() {
    let mesh = MockMeshDevice::new(MockMode::Source);

    mesh.inject_packet(b"from-mesh".to_vec()).await;

    let packet = timeout(Duration::from_millis(100), mesh.recv_raw_packet())
        .await
        .expect("timed out")
        .unwrap();

    assert_eq!(packet, b"from-mesh");
}

// Source mode: send_packet is silently dropped, only inject→recv works
#[tokio::test]
async fn test_source_mode_ignores_sends() {
    let mesh = Arc::new(MockMeshDevice::new(MockMode::Source)) as Arc<dyn MeshDevice + Send + Sync>;

    // This should not panic or error, just be silently dropped
    mesh.send_packet(b"ignored").await.unwrap();

    // Pre-load a real packet and verify it comes through unaffected
    let mesh_concrete = MockMeshDevice::new(MockMode::Source);
    mesh_concrete.inject_packet(b"real".to_vec()).await;
    let packet = timeout(Duration::from_millis(100), mesh_concrete.recv_raw_packet())
        .await
        .expect("timed out")
        .unwrap();
    assert_eq!(packet, b"real");
}
