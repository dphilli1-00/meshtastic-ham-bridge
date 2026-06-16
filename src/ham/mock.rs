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
    tx: mpsc::Sender<Vec<u8>>,
    rx: Mutex<mpsc::Receiver<Vec<u8>>>,
}

#[async_trait]
impl HamDevice for MockHamDevice {
    fn device_type(&self) -> &str {
        "mock-ham"
    }

    async fn send_frame(&self, data: &[u8]) -> HamResult<()> {
        match self.mode {
            MockMode::Echo => { self.tx.send(data.to_vec()).await.ok(); }
            MockMode::Sink => {}  // drop it
            MockMode::Source => {}  // ignore sends
        }
        Ok(())
    }


    async fn recv_frame(&self) -> HamResult<Vec<u8>> {
        let mut rx = self.rx.lock().await;
        rx.recv().await.ok_or(crate::error::Error::Unknown("channel closed".into()))
    }

    async fn status(&self) -> HamResult<DeviceStatus> {
        todo!()  
    }
    async fn disconnect(&self) -> HamResult<()> {
         Ok(())
    }

    // ... status, disconnect, send_text
}