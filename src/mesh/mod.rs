use async_trait::async_trait;
use crate::error::Result as MeshResult;
use crate::types::DeviceStatus;

#[async_trait]
pub trait MeshDevice {
    fn device_type(&self) -> &str;
    
    async fn send_text(&self, text: &str) -> MeshResult<()>;
    async fn send_packet(&self, data: &[u8]) -> MeshResult<()>;
    async fn status(&self) -> MeshResult<DeviceStatus>;
    async fn disconnect(&self) -> MeshResult<()>;
    async fn recv_raw_packet(&self) -> MeshResult<Vec<u8>>;
}


