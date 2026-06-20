package mesh

// MeshtasticBLEDevice connects to a Meshtastic node over BLE.
//
// Meshtastic BLE protocol:
//   Service:   6ba1b218-15a8-461f-9fa8-5d6646720a11
//   ToRadio:   f75c76d2-129e-4dad-a1dd-7866124401e7  (write — send packets to node)
//   FromRadio: 2c55cbb9-c8ae-4bbd-be89-9a6b2e0787ad  (read — fetch packet from node)
//   FromNum:   ed9da18c-a800-4f66-a670-aa7547b43d82  (notify — signals a new FromRadio packet)
//
// Flow:
//   1. Connect + pair
//   2. Discover service + characteristics
//   3. Enable notifications on FromNum
//   4. On FromNum notify → read FromRadio → push to recv channel
//   5. SendPacket → write to ToRadio

import (
	"context"
	"fmt"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

var (
	meshtasticServiceUUID = bluetooth.NewUUID([16]byte{
		0x6b, 0xa1, 0xb2, 0x18, 0x15, 0xa8, 0x46, 0x1f,
		0x9f, 0xa8, 0x5d, 0xca, 0xe2, 0x73, 0xea, 0xfd,
	})
	toRadioUUID = bluetooth.NewUUID([16]byte{
		0xf7, 0x5c, 0x76, 0xd2, 0x12, 0x9e, 0x4d, 0xad,
		0xa1, 0xdd, 0x78, 0x66, 0x12, 0x44, 0x01, 0xe7,
	})
	fromRadioUUID = bluetooth.NewUUID([16]byte{
		0x2c, 0x55, 0xe6, 0x9e, 0x49, 0x93, 0x11, 0xed,
		0xb8, 0x78, 0x02, 0x42, 0xac, 0x12, 0x00, 0x02,
	})
	fromNumUUID = bluetooth.NewUUID([16]byte{
		0xed, 0x9d, 0xa1, 0x8c, 0xa8, 0x00, 0x4f, 0x66,
		0xa6, 0x70, 0xaa, 0x75, 0x47, 0xe3, 0x44, 0x53,
	})
)

// MeshtasticBLEDevice implements mesh.Device over BLE.
type MeshtasticBLEDevice struct {
	device    bluetooth.Device
	toRadio   bluetooth.DeviceCharacteristic
	fromRadio bluetooth.DeviceCharacteristic
	recv      chan []byte
	done      chan struct{}
}

// ConnectMeshtasticBLE connects to a Meshtastic node by BLE MAC address.
// addr format: "C0:C2:24:70:D8:15"
func ConnectMeshtasticBLE(addr string) (*MeshtasticBLEDevice, error) {
	// Ensure paired before GATT — Windows needs explicit pairing to get encryption.
	if err := pairBLEDevice(addr); err != nil {
		return nil, fmt.Errorf("BLE pair: %w", err)
	}

	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("BLE enable: %w", err)
	}

	// Parse the target address
	var targetAddr bluetooth.Address
	targetAddr.Set(addr)

	// Connect directly by address — no scan needed.
	// Scanning fails when the device is already connected to another host (e.g. Chrome WebBluetooth)
	// because connected devices stop advertising. Windows WinRT supports direct address connect.
	dev, err := adapter.Connect(targetAddr, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("BLE connect %s: %w", addr, err)
	}

	// Discover all services first so we can diagnose UUID mismatches
	allServices, err := dev.DiscoverServices(nil)
	if err != nil {
		dev.Disconnect()
		return nil, fmt.Errorf("BLE: service discovery failed on %s: %w", addr, err)
	}

	var svc bluetooth.DeviceService
	found2 := false
	for _, s := range allServices {
		if s.UUID() == meshtasticServiceUUID {
			svc = s
			found2 = true
			break
		}
	}
	if !found2 {
		uuids := make([]string, len(allServices))
		for i, s := range allServices {
			uuids[i] = s.UUID().String()
		}
		dev.Disconnect()
		return nil, fmt.Errorf("BLE: Meshtastic service not found on %s\n  found services: %v", addr, uuids)
	}

	// Discover all characteristics so we can diagnose UUID mismatches
	chars, err := svc.DiscoverCharacteristics(nil)
	if err != nil {
		dev.Disconnect()
		return nil, fmt.Errorf("BLE: characteristic discovery failed: %w", err)
	}
	if len(chars) == 0 {
		dev.Disconnect()
		return nil, fmt.Errorf("BLE: no characteristics found in Meshtastic service")
	}
	// Log what we found
	for _, c := range chars {
		fmt.Printf("  characteristic: %s\n", c.UUID().String())
	}

	// Map by UUID
	charMap := make(map[bluetooth.UUID]bluetooth.DeviceCharacteristic, len(chars))
	for _, c := range chars {
		charMap[c.UUID()] = c
	}
	toRadio, ok1 := charMap[toRadioUUID]
	fromRadio, ok2 := charMap[fromRadioUUID]
	fromNum, ok3 := charMap[fromNumUUID]
	if !ok1 || !ok2 || !ok3 {
		dev.Disconnect()
		return nil, fmt.Errorf("BLE: missing characteristics — toRadio:%v fromRadio:%v fromNum:%v", ok1, ok2, ok3)
	}

	d := &MeshtasticBLEDevice{
		device:    dev,
		toRadio:   toRadio,
		fromRadio: fromRadio,
		recv:      make(chan []byte, 32),
		done:      make(chan struct{}),
	}

	// Subscribe to FromNum notifications — each notify means a new packet is ready.
	// Falls back to polling if the device requires bonding before notifications.
	notifyErr := fromNum.EnableNotifications(func(buf []byte) {
		fmt.Printf("BLE: FromNum notify fired, value=% X\n", buf)
		d.fetchFromRadio()
	})
	if notifyErr != nil {
		fmt.Printf("BLE: FromNum notify unavailable (%v)\n", notifyErr)
	} else {
		fmt.Println("BLE: FromNum notifications enabled")
	}
	// Always poll as safety net — notifications may not fire on all platforms/bonds
	go d.pollLoop()

	return d, nil
}

// pollLoop reads FromRadio on a timer — fallback when notifications are unavailable.
func (d *MeshtasticBLEDevice) pollLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
			ticks++
			if ticks == 1 || ticks%20 == 0 { // log first tick and every 2s
				fmt.Printf("BLE poll tick %d\n", ticks)
			}
			d.fetchFromRadio()
		}
	}
}

// fetchFromRadio drains all pending packets from FromRadio.
// Meshtastic queues multiple packets after WantConfigId — read until empty.
func (d *MeshtasticBLEDevice) fetchFromRadio() {
	buf := make([]byte, 512)
	for {
		n, err := d.fromRadio.Read(buf)
		if err != nil {
			fmt.Printf("BLE fromRadio read err: %v\n", err)
			return
		}
		if n == 0 {
			return // no more packets queued
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		select {
		case d.recv <- pkt:
		case <-d.done:
			return
		}
	}
}

func (d *MeshtasticBLEDevice) DeviceType() string { return "meshtastic-ble" }

func (d *MeshtasticBLEDevice) SendText(ctx context.Context, text string) error {
	return d.SendPacket(ctx, []byte(text))
}

func (d *MeshtasticBLEDevice) SendPacket(_ context.Context, data []byte) error {
	// TODO: wrap in Meshtastic protobuf ToRadio envelope (task #25)
	_, err := d.toRadio.Write(data)
	return err
}

func (d *MeshtasticBLEDevice) RecvPacket(ctx context.Context) ([]byte, error) {
	select {
	case pkt, ok := <-d.recv:
		if !ok {
			return nil, fmt.Errorf("meshtastic BLE: connection closed")
		}
		return pkt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *MeshtasticBLEDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (d *MeshtasticBLEDevice) Close() error {
	close(d.done)
	return d.device.Disconnect()
}
