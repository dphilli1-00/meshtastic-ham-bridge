package types

import "time"

// Packet is a raw packet moving through the bridge.
type Packet struct {
	ID        string
	Timestamp time.Time
	Source    PacketSource
	Kind      PacketType
	Data      []byte
	Text      string
}

type PacketSource int

const (
	SourceMeshtastic PacketSource = iota
	SourceMeshcore
	SourceDirewolf
	SourceBridge
)

type PacketType int

const (
	PacketText PacketType = iota
	PacketAPRS
	PacketAX25
	PacketTelemetry
	PacketOther
)

// DeviceStatus is returned by Status() on any device.
type DeviceStatus struct {
	Connected   bool
	BatteryPct  *int // nil if unknown
	IsCharging  *bool
	NodeID      *uint32
	NodeName    string
}

func Connected() DeviceStatus    { return DeviceStatus{Connected: true} }
func Disconnected() DeviceStatus { return DeviceStatus{Connected: false} }
