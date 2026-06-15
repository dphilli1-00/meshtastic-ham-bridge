use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("Device not connected: {0}")]
    DeviceNotConnected(String),
    
    #[error("Serial port error: {0}")]
    SerialPort(String),
    
    #[error("BLE error: {0}")]
    Ble(String),
    
    #[error("Invalid packet: {0}")]
    InvalidPacket(String),
    
    #[error("Unknown error: {0}")]
    Unknown(String),
}

pub type Result<T> = std::result::Result<T, Error>;