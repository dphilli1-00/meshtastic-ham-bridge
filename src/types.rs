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
    connected: bool,
battery_level: Option<u8>,
is_charging: Option<bool>,
node_id: Option<u32>,
node_name: Option<String>
}
