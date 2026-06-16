use async_trait::async_trait;
use crate::error::Result as HamResult;
use crate::types::DeviceStatus;
pub mod mock;
#[async_trait]
pub trait HamDevice {
    fn device_type(&self) -> &str;
    
    async fn send_frame(&self, data: &[u8]) -> HamResult<()>;
    async fn recv_frame(&self) -> HamResult<Vec<u8>>;
    async fn status(&self) -> HamResult<DeviceStatus>;
    async fn disconnect(&self) -> HamResult<()>;

}