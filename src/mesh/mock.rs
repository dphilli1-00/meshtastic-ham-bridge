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
    // outgoing: send_packet pushes here, bridge reads via recv_raw_packet in Echo mode
    out_tx: mpsc::Sender<Vec<u8>>,
    out_rx: Mutex<mpsc::Receiver<Vec<u8>>>,
    // incoming: bridge injects here via send_packet in Sink mode, test reads via recv_raw_packet
    in_tx: mpsc::Sender<Vec<u8>>,
    in_rx: Mutex<mpsc::Receiver<Vec<u8>>>,
}

impl MockMeshDevice {
    pub fn new(mode: MockMode) -> Self {
        let (out_tx, out_rx) = mpsc::channel(32);
        let (in_tx, in_rx) = mpsc::channel(32);
        Self {
            mode,
            out_tx,
            out_rx: Mutex::new(out_rx),
            in_tx,
            in_rx: Mutex::new(in_rx),
        }
    }

    // called by bridge to inject a packet received from ham side
    pub async fn inject_packet(&self, data: Vec<u8>) {
        self.in_tx.send(data).await.ok();
    }
}

#[async_trait]
impl MeshDevice for MockMeshDevice {
    fn device_type(&self) -> &str {
        "mock-mesh"
    }

    async fn send_packet(&self, data: &[u8]) -> MeshResult<()> {
        match self.mode {
            MockMode::Echo => { self.out_tx.send(data.to_vec()).await.ok(); } // echoes back locally
            MockMode::Sink => { self.in_tx.send(data.to_vec()).await.ok(); }  // bridge injects here
            MockMode::Source => {}  // ignores sends
        }
        Ok(())
    }

    async fn recv_raw_packet(&self) -> MeshResult<Vec<u8>> {
        match self.mode {
            MockMode::Echo => {
                let mut rx = self.out_rx.lock().await;
                rx.recv().await.ok_or(crate::error::Error::Unknown("channel closed".into()))
            }
            MockMode::Sink => {
                let mut rx = self.in_rx.lock().await;
                rx.recv().await.ok_or(crate::error::Error::Unknown("channel closed".into()))
            }
            MockMode::Source => {
                // source: test pre-loads packets via inject_packet(), bridge reads them out
                let mut rx = self.in_rx.lock().await;
                rx.recv().await.ok_or(crate::error::Error::Unknown("channel closed".into()))
            }
        }
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