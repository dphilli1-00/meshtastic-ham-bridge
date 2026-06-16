use tokio::sync::{mpsc, Mutex};
use async_trait::async_trait;
use crate::error::Result as MeshResult;
use crate::types::DeviceStatus;
use super::MeshDevice;  // pulls in the trait from mod.rs

pub enum MockMode {  // rename MockNode → MockMode, more descriptive
    Source,
    Sink,
    Echo,
}

pub struct MockMeshDevice {
    mode: MockMode,
    tx: mpsc::Sender<Vec<u8>>,
    rx: Mutex<mpsc::Receiver<Vec<u8>>>,
}

impl MockMeshDevice {
    pub fn new(mode: MockMode) -> Self {
        let (tx, rx) = mpsc::channel(32);  // 32 = buffer size
        Self {
            mode,
            tx,
            rx: Mutex::new(rx),
        }
    }
}

#[async_trait]
impl MeshDevice for MockMeshDevice {
    fn device_type(&self) -> &str {
        "mock-mesh"
    }

    async fn send_packet(&self, data: &[u8]) -> MeshResult<()> {
        match self.mode {
            MockMode::Echo => { self.tx.send(data.to_vec()).await.ok(); }
            MockMode::Sink => {}  // drop it
            MockMode::Source => {}  // ignore sends
        }
        Ok(())
    }

    async fn recv_raw_packet(&self) -> MeshResult<Vec<u8>> {
        let mut rx = self.rx.lock().await;
        rx.recv().await.ok_or(crate::error::Error::Unknown("channel closed".into()))
    }

    async fn status(&self) -> MeshResult<DeviceStatus> {
        todo!()  
    }
    async fn disconnect(&self) -> MeshResult<()> {
         Ok(())
    }
    async fn send_text(&self, text: &str) -> MeshResult<()> {
        self.send_packet(text.as_bytes()).await
    }

    // ... status, disconnect, send_text
}