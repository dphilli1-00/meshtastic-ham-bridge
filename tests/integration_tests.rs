use std::sync::Arc;
use meshtastic_ham_bridge::mesh::mock::{MockMeshDevice, MockMode as MeshMockMode};
use meshtastic_ham_bridge::ham::mock::{MockHamDevice, MockMode as HamMockMode};
use meshtastic_ham_bridge::bridge::Bridge;
use meshtastic_ham_bridge::mesh::MeshDevice;
use meshtastic_ham_bridge::ham::HamDevice;
use tokio::time::{timeout, Duration};

#[tokio::test]
async fn test_mesh_echo() {
    let mesh = Arc::new(MockMeshDevice::new(MeshMockMode::Echo)) as Arc<dyn MeshDevice + Send + Sync>;
    let ham = Arc::new(MockHamDevice::new(HamMockMode::Sink)) as Arc<dyn HamDevice + Send + Sync>;
    let mesh_ref = Arc::clone(&mesh);
    let bridge = Bridge::new(Arc::clone(&mesh), Arc::clone(&ham));
    bridge.run().await;

    mesh_ref.send_text("foo").await.unwrap();

    let packet = timeout(Duration::from_millis(100), mesh_ref.recv_raw_packet())
        .await
        .expect("timed out")
        .unwrap();

    println!("received: {:?}", packet);
    assert_eq!(packet, b"foo");
}

#[tokio::test]
async fn test_bridge_full_path() {
    let mesh = Arc::new(MockMeshDevice::new(MeshMockMode::Sink)) as Arc<dyn MeshDevice + Send + Sync>;
    let ham = Arc::new(MockHamDevice::new(HamMockMode::Echo)) as Arc<dyn HamDevice + Send + Sync>;
    let mesh_ref = Arc::clone(&mesh);
    let bridge = Bridge::new(Arc::clone(&mesh), Arc::clone(&ham));
    bridge.run().await;

    tokio::time::sleep(Duration::from_millis(10)).await;

    mesh_ref.send_text("bar").await.unwrap();

    let packet = timeout(Duration::from_millis(200), mesh_ref.recv_raw_packet())
        .await
        .expect("timed out waiting for packet through bridge")
        .unwrap();

    println!("received: {:?}", packet);
    assert_eq!(packet, b"bar");
}
