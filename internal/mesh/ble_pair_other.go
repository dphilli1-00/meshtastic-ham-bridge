//go:build !windows

package mesh

// pairBLEDevice is a no-op on non-Windows — BlueZ and CoreBluetooth handle
// pairing automatically when GATT operations require it.
func pairBLEDevice(macAddr string) error {
	return nil
}
