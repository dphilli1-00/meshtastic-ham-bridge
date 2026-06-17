use tokio::sync::{mpsc, Mutex};
use async_trait::async_trait;
use crate::error::Result as HamResult;
use crate::types::DeviceStatus;
use super::HamDevice;  // pulls in the trait from mod.rs

pub enum MockMode {  // rename MockNode → MockMode, more descriptive
    Source,
    Sink,
    Echo,
}

pub struct MockHamDevice {
    mode: MockMode,
    // outgoing: send_frame pushes here, bridge reads via recv_frame in Echo mode
    out_tx: mpsc::Sender<Vec<u8>>,
    out_rx: Mutex<mpsc::Receiver<Vec<u8>>>,
    // incoming: bridge injects here via send_frame in Sink mode
    in_tx: mpsc::Sender<Vec<u8>>,
    in_rx: Mutex<mpsc::Receiver<Vec<u8>>>,
}

impl MockHamDevice {
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

    pub async fn inject_frame(&self, data: Vec<u8>) {
        self.in_tx.send(data).await.ok();
    }
}

#[async_trait]
impl HamDevice for MockHamDevice {
    fn device_type(&self) -> &str {
        "mock-ham"
    }

    async fn send_frame(&self, data: &[u8]) -> HamResult<()> {
        match self.mode {
            MockMode::Echo => { self.out_tx.send(data.to_vec()).await.ok(); } // echoes back
            MockMode::Sink => { self.in_tx.send(data.to_vec()).await.ok(); }  // bridge injects here
            MockMode::Source => {}  // ignores sends
        }
        Ok(())
    }

    async fn recv_frame(&self) -> HamResult<Vec<u8>> {
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
                todo!()
            }
        }
    }

    async fn status(&self) -> HamResult<DeviceStatus> {
        todo!()  
    }
    async fn disconnect(&self) -> HamResult<()> {
         Ok(())
    }

    // ... status, disconnect, send_text
}