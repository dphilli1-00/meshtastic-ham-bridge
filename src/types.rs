use serde::{Deserialize, Serialize};


#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Packet {
    id: String,
    timestamp: chrono::DateTime<chrono::Utc>,
    source: PacketSource,
    packet_type: PacketType,
    data: Vec<u8>,
    text: Option<String>}


#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum PacketSource {
    Meshtastic,
    Meshcore,
    Direwolf,
    Bridge
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum PacketType {
    Text,
Aprs,
Ax25,
Telemetry,
Other(String)
}
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DeviceStatus {
    pub connected: bool,
    pub battery_level: Option<u8>,
    pub is_charging: Option<bool>,
    pub node_id: Option<u32>,
    pub node_name: Option<String>,
}

impl DeviceStatus {
    pub fn connected() -> Self {
        Self { connected: true, battery_level: None, is_charging: None, node_id: None, node_name: None }
    }
    pub fn disconnected() -> Self {
        Self { connected: false, battery_level: None, is_charging: None, node_id: None, node_name: None }
    }
}
