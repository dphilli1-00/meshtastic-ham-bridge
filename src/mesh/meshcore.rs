use meshcore_rs::MeshCore;
use tokio::sync::Mutex;
use async_trait::async_trait;
use crate::error::Result as MeshResult;
use crate::types::DeviceStatus;
use super::MeshDevice;

pub struct MeshcoreDevice {
    core: Mutex<MeshCore>,
}

impl MeshcoreDevice {
    pub async fn connect_serial(port: &str, baud: u32) -> MeshResult<Self> {
        let core = MeshCore::serial(port, baud).await
            .map_err(|e| crate::error::Error::Unknown(e.to_string()))?;
        Ok(Self { core: Mutex::new(core) })
    }

    pub async fn connect_ble(name: &str) -> MeshResult<Self> {
        let core = MeshCore::ble_connect(name).await
            .map_err(|e| crate::error::Error::Ble(e.to_string()))?;
        Ok(Self { core: Mutex::new(core) })
    }
}

#[async_trait]
impl MeshDevice for MeshcoreDevice {
    fn device_type(&self) -> &str { "meshcore-serial" }
    async fn send_packet(&self, data: &[u8]) -> MeshResult<()> { todo!() }
    async fn recv_raw_packet(&self) -> MeshResult<Vec<u8>> { todo!() }
    async fn send_text(&self, text: &str) -> MeshResult<()> { todo!() }
    async fn status(&self) -> MeshResult<DeviceStatus> { todo!() }
    async fn disconnect(&self) -> MeshResult<()> { todo!() }
}