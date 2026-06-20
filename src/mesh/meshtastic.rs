use meshtastic::api::{StreamApi, StreamHandle};
use meshtastic::utils::stream::build_serial_stream;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::Mutex;
use async_trait::async_trait;
use crate::error::Result as MeshResult;
use crate::types::DeviceStatus;
use super::MeshDevice;

pub struct MeshtasticDevice {
    api: Mutex<meshtastic::api::ConnectedStreamApi<meshtastic::api::state::Configured>>,
    receiver: Mutex<meshtastic::packet::PacketReceiver>,
}

impl MeshtasticDevice {
    // Generic connect — accepts any AsyncRead+AsyncWrite stream (serial, TCP, duplex for tests)
    pub async fn connect<T>(stream: T) -> MeshResult<Self>
    where
        T: AsyncRead + AsyncWrite + Send + Unpin + 'static,
    {
        let handle = StreamHandle::from_stream(stream);
        let (receiver, api) = StreamApi::new().connect(handle).await;
        let api = api.configure(meshtastic::utils::generate_rand_id()).await
            .map_err(|e| crate::error::Error::Unknown(e.to_string()))?;
        Ok(Self {
            api: Mutex::new(api),
            receiver: Mutex::new(receiver),
        })
    }

    // Serial port convenience constructor
    pub async fn connect_serial(port: &str) -> MeshResult<Self> {
        let handle = build_serial_stream(port.to_string(), None, None, None)
            .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;
        // build_serial_stream returns a StreamHandle directly
        let (receiver, api) = StreamApi::new().connect(handle).await;
        let api = api.configure(meshtastic::utils::generate_rand_id()).await
            .map_err(|e| crate::error::Error::Unknown(e.to_string()))?;
        Ok(Self {
            api: Mutex::new(api),
            receiver: Mutex::new(receiver),
        })
    }
}

#[async_trait]
impl MeshDevice for MeshtasticDevice {
    fn device_type(&self) -> &str { "meshtastic-serial" }

    async fn send_packet(&self, _data: &[u8]) -> MeshResult<()> {
        todo!()
    }

    async fn recv_raw_packet(&self) -> MeshResult<Vec<u8>> {
        todo!()
    }

    async fn send_text(&self, _text: &str) -> MeshResult<()> {
        todo!()
    }

    async fn status(&self) -> MeshResult<DeviceStatus> { todo!() }
    async fn disconnect(&self) -> MeshResult<()> { todo!() }
}