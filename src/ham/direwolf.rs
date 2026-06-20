use tokio::net::TcpStream;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::sync::Mutex;
use async_trait::async_trait;
use crate::error::Result as HamResult;
use crate::types::DeviceStatus;
use super::HamDevice;

pub struct DirewolfDevice {
    host: String,
    port: u16,
    stream: Mutex<TcpStream>,
}

impl DirewolfDevice {
    pub async fn connect(host: &str, port: u16) -> HamResult<Self> {
        let stream = TcpStream::connect(format!("{}:{}", host, port)).await
            .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;
        Ok(Self {
            host: host.to_string(),
            port,
            stream: Mutex::new(stream),
        })
    }
}

#[async_trait]
impl HamDevice for DirewolfDevice {
    fn device_type(&self) -> &str {
         "direwolf-kiss-tcp"
        }
    async fn send_frame(&self, data: &[u8]) -> HamResult<()> {
        let mut frame = vec![0xC0u8, 0x00]; // start + command byte
    
        for &byte in data {
            match byte {
                0xC0 => { frame.push(0xDB); frame.push(0xDC); }
                0xDB => { frame.push(0xDB); frame.push(0xDD); }
                b => frame.push(b),
            }
        }
        
        frame.push(0xC0); // end
        
        let mut stream = self.stream.lock().await;
        stream.write_all(&frame).await
            .map_err(|e| crate::error::Error::SerialPort(e.to_string()))
        }

    async fn recv_frame(&self) -> HamResult<Vec<u8>> {
        let mut stream = self.stream.lock().await;
        let mut buffer: Vec<u8> = Vec::new();
    
        // skip bytes until we see the start 0xC0
        loop {
            let byte = stream.read_u8().await
                .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;
            if byte == 0xC0 { break; }
        }
        // skip command byte (outside the payload loop)
        let _cmd = stream.read_u8().await
            .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;

         // read payload until end 0xC0
        loop {
            let byte = stream.read_u8().await
                .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;
            match byte {
                0xC0 => break, // end of frame
                0xDB => {
                    // escape sequence
                    let next = stream.read_u8().await
                        .map_err(|e| crate::error::Error::SerialPort(e.to_string()))?;
                    match next {
                        0xDC => buffer.push(0xC0),
                        0xDD => buffer.push(0xDB),
                        _ => return Err(crate::error::Error::InvalidPacket("bad escape".into())),
                    }
                }
                b => buffer.push(b),
            }
        }        

        Ok(buffer)
        }
    async fn status(&self) -> HamResult<DeviceStatus> {
         todo!()
         }
    async fn disconnect(&self) -> HamResult<()> {
         todo!()
         }
}


