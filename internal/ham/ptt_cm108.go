//go:build !android && !ios

package ham

// CM108PTT controls PTT via the CM108 HID GPIO interface.
//
// The CM108 chip (used in Digirig and most cheap USB audio dongles for radio)
// exposes a HID interface in addition to the USB audio interface. GPIO4 on the
// chip is wired to the radio's PTT line.
//
// PTT is controlled by sending a 4-byte HID output report:
//   Byte 0: 0x00  (report ID / unused)
//   Byte 1: GPIO values  (0x08 = GPIO4 high = PTT active)
//   Byte 2: GPIO directions (0x08 = GPIO4 as output)
//   Byte 3: 0x00  (reserved)
//
// Finding the HID path:
//   Windows: run cm108.exe or Device Manager → HID devices → details
//            Looks like: \\?\hid#vid_0d8c&pid_0012&mi_03#7&...
//   Linux:   /dev/hidrawN  (find with: ls -la /dev/hidraw* after plugging in)
//   macOS:   use FindCM108() to locate by VID/PID automatically
//
// The HID path is NOT stable across USB replugs on Windows.
// On Linux/macOS the VID/PID search via FindCM108() is more robust.

import (
	"fmt"

	"github.com/karalabe/hid"
)

const (
	cm108VendorID  = 0x0d8c // C-Media Electronics
	cm108GPIO4Mask = 0x08   // GPIO4 bit
)

// CM108PTT opens a CM108 HID device by path and controls PTT via GPIO4.
type CM108PTT struct {
	dev *hid.Device
}

// NewCM108PTT opens a CM108 HID device for PTT.
// path: HID device path (Windows: full \\?\hid#... string; Linux: /dev/hidrawN).
//       Pass "" to auto-detect the first CM108 by VID/PID.
func NewCM108PTT(path string) (*CM108PTT, error) {
	devs := hid.Enumerate(cm108VendorID, 0) // 0 = any PID
	if len(devs) == 0 {
		return nil, fmt.Errorf("no CM108 HID device found (VID 0x%04x)", cm108VendorID)
	}

	var info *hid.DeviceInfo
	if path == "" {
		// Auto-detect: prefer interface 3 (CM108 GPIO HID interface).
		for i := range devs {
			if devs[i].Interface == 3 {
				info = &devs[i]
				break
			}
		}
		if info == nil {
			info = &devs[0]
		}
	} else {
		for i := range devs {
			if devs[i].Path == path {
				info = &devs[i]
				break
			}
		}
		if info == nil {
			return nil, fmt.Errorf("CM108 HID device not found at path %q", path)
		}
	}

	dev, err := info.Open()
	if err != nil {
		return nil, fmt.Errorf("CM108 open %s: %w", info.Path, err)
	}
	fmt.Printf("CM108 PTT: opened %s (VID=%04x PID=%04x)\n", info.Path, info.VendorID, info.ProductID)
	return &CM108PTT{dev: dev}, nil
}

// ListCM108 prints all detected CM108 HID devices. Useful for finding the right path.
func ListCM108() {
	devs := hid.Enumerate(cm108VendorID, 0)
	if len(devs) == 0 {
		fmt.Println("No CM108 HID devices found")
		return
	}
	for _, d := range devs {
		fmt.Printf("CM108: VID=%04x PID=%04x Interface=%d UsagePage=%04x\n  Path: %s\n",
			d.VendorID, d.ProductID, d.Interface, d.UsagePage, d.Path)
	}
}

// On activates PTT (GPIO4 high).
func (p *CM108PTT) On() error {
	return p.sendGPIO(cm108GPIO4Mask)
}

// Off deactivates PTT (GPIO4 low).
func (p *CM108PTT) Off() error {
	return p.sendGPIO(0x00)
}

// sendGPIO sends a CM108 HID GPIO report.
// val: GPIO4..1 values (bit 3 = GPIO4)
func (p *CM108PTT) sendGPIO(val byte) error {
	// 5-byte write: report ID (0) + 4 payload bytes.
	// karalabe/hid prepends the report ID automatically on Windows;
	// we send 4 bytes and it handles the framing.
	report := []byte{
		0x00,            // report ID
		val,             // GPIO values: bit3=GPIO4
		cm108GPIO4Mask,  // GPIO directions: GPIO4 = output
		0x00,            // reserved
	}
	_, err := p.dev.Write(report)
	return err
}

func (p *CM108PTT) Close() error {
	return p.dev.Close()
}
