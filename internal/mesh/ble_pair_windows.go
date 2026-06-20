//go:build windows

package mesh

// pairBLEDevice checks whether the device is already bonded to this PC via
// Windows BLE. If paired, returns nil immediately. If not paired, returns a
// descriptive error telling the user to pair once via the Meshtastic app.
//
// Flow:
//   BluetoothLEDeviceFromBluetoothAddressAsync
//     → iBluetoothLEDevice2.GetDeviceInformation
//     → IDeviceInformation2.get_Pairing → IsPaired check

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/go-ole/go-ole"
	winrtble "github.com/saltosystems/winrt-go/windows/devices/bluetooth"
)

// pairBLEDevice ensures the device is bonded to Windows before connecting.
// If already paired, returns immediately. Otherwise opens a local browser page
// that uses Web Bluetooth to bond the device, then returns.
// Safe to call if already paired.
func pairBLEDevice(macAddr string) error {
	// Initialize COM for the duration of this function, then uninitialize so
	// tinygo's adapter.Enable() can set up its own WinRT apartment cleanly.
	// S_FALSE (1) = already initialized by another caller — both are fine.
	ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED) //nolint:errcheck
	defer ole.CoUninitialize()

	addrInt, err := macStringToUint64(macAddr)
	if err != nil {
		return fmt.Errorf("BLE pair: invalid MAC %q: %w", macAddr, err)
	}

	// 1. Get BluetoothLEDevice from address.
	devOp, err := winrtble.BluetoothLEDeviceFromBluetoothAddressAsync(addrInt)
	if err != nil {
		return fmt.Errorf("BLE: FromBluetoothAddressAsync: %w", err)
	}
	defer devOp.Release()

	devPtr, err := awaitAsyncOp(devOp, 10_000_000_000 /* 10s */)
	if err != nil {
		return fmt.Errorf("BLE: await device: %w", err)
	}
	if devPtr == nil {
		return fmt.Errorf("BLE: device %s not found — is it in range?", macAddr)
	}
	bleDevUnk := (*ole.IUnknown)(devPtr)
	defer bleDevUnk.Release()

	// 2. QI for iBluetoothLEDevice2 to reach DeviceInformation.
	ble2Itf, err := bleDevUnk.QueryInterface(ole.NewGUID(guidIBluetoothLEDevice2Enum))
	if err != nil {
		return fmt.Errorf("BLE: QI iBluetoothLEDevice2: %w", err)
	}
	ble2 := (*iBLEDevice2ForPairing)(unsafe.Pointer(ble2Itf))
	defer ble2.Release()

	partialDevInfoUnk, err := ble2.getDeviceInformation()
	if err != nil {
		return fmt.Errorf("BLE: GetDeviceInformation: %w", err)
	}
	defer partialDevInfoUnk.Release()

	// The object from BluetoothLEDevice.DeviceInformation is a lightweight cached
	// object that only implements IDeviceInformation v1 (no Pairing support).
	// Get the device ID string from it, then call CreateFromIdAsync to get a full
	// DeviceInformation object that supports IDeviceInformation2 + Pairing.
	partialV1Itf, err := partialDevInfoUnk.QueryInterface(ole.NewGUID(guidIDeviceInformationV1))
	if err != nil {
		return fmt.Errorf("BLE: QI IDeviceInformation v1: %w", err)
	}
	partialV1 := (*iDeviceInformationV1)(unsafe.Pointer(partialV1Itf))
	deviceId, err := partialV1.getId()
	partialV1.Release()
	if err != nil {
		return fmt.Errorf("BLE: get_Id: %w", err)
	}
	fmt.Printf("BLE: device ID = %s\n", deviceId)

	// 3. Get full DeviceInformation via CreateFromIdAsync.
	fullDevOp, err := deviceInformationCreateFromIdAsync(deviceId)
	if err != nil {
		return fmt.Errorf("BLE: CreateFromIdAsync: %w", err)
	}
	defer fullDevOp.Release()

	fullDevPtr, err := awaitAsyncOp(fullDevOp, 10_000_000_000)
	if err != nil {
		return fmt.Errorf("BLE: await full DeviceInformation: %w", err)
	}
	if fullDevPtr == nil {
		return fmt.Errorf("BLE: CreateFromIdAsync returned nil")
	}
	fullDevUnk := (*ole.IUnknown)(fullDevPtr)
	defer fullDevUnk.Release()

	// Diagnostic: dump IIDs of the full object to confirm it has IDeviceInformation2.
	if iids, iidErr := winrtGetIIDs(fullDevUnk); iidErr == nil {
		fmt.Printf("BLE: full DeviceInformation IIDs (%d):\n", len(iids))
		for _, id := range iids {
			fmt.Printf("  %s\n", id)
		}
	}

	// 4. Find IDeviceInformation2 — try each observed IID, validate by checking
	// that slot 7 (get_Pairing) returns a DeviceInformationPairing runtime class.
	var pairing *iDeviceInformationPairing
	for _, candidateGUID := range []string{
		guidIDeviceInformation2,                 // {F156A638} confirmed at runtime
		"2743208b-aff1-4038-be7a-93daed1f0687", // IID #4 observed in the wild
	} {
		itf, qiErr := fullDevUnk.QueryInterface(ole.NewGUID(candidateGUID))
		if qiErr != nil {
			fmt.Printf("BLE: QI {%s}: %v\n", candidateGUID, qiErr)
			continue
		}
		candidate := (*iDeviceInformation2)(unsafe.Pointer(itf))
		p, name, probeErr := probeDevInfo2GetPairing(candidate)
		candidate.Release()
		if probeErr != nil {
			fmt.Printf("BLE: {%s} slot7 → %v\n", candidateGUID, probeErr)
			continue
		}
		fmt.Printf("BLE: {%s} slot7 → %q\n", candidateGUID, name)
		if name != "Windows.Devices.Enumeration.DeviceInformationPairing" {
			(*ole.IUnknown)(unsafe.Pointer(p)).Release()
			continue
		}
		fmt.Printf("BLE: IDeviceInformation2 GUID = {%s} ✓\n", candidateGUID)
		pairing = p
		break
	}
	if pairing == nil {
		return fmt.Errorf("BLE: could not locate IDeviceInformation2/get_Pairing on DeviceInformation object")
	}
	defer pairing.Release()

	// Diagnose the pairing object before using it.
	pairingUnk := (*ole.IUnknown)(unsafe.Pointer(pairing))
	if name, nerr := winrtRuntimeClassName(pairingUnk); nerr == nil {
		fmt.Printf("BLE: pairing RuntimeClassName = %q\n", name)
	}
	if iids, iidErr := winrtGetIIDs(pairingUnk); iidErr == nil {
		fmt.Printf("BLE: pairing IIDs (%d):\n", len(iids))
		for _, id := range iids {
			fmt.Printf("  %s\n", id)
		}
	}

	// 5. Already paired? We're done.
	isPaired, err := pairing.getIsPaired()
	if err != nil {
		return fmt.Errorf("BLE: IsPaired: %w", err)
	}
	if isPaired {
		return nil
	}

	// 6. Not paired — instruct the user to pair via the Meshtastic app.
	// Windows BLE pairing with a PIN requires a COM delegate callback that
	// can't be safely implemented in pure Go against the WinRT ABI.
	// Pair once via the Meshtastic mobile app or web client, then re-run.
	return fmt.Errorf(
		"BLE: %s is not paired to this PC.\n"+
			"  Pair it once using the Meshtastic app (Android/iOS) or web client\n"+
			"  at https://client.meshtastic.org, then re-run this command.\n"+
			"  Once paired, this tool connects automatically on every subsequent run.",
		macAddr,
	)
}

// macStringToUint64 converts "C0:C2:24:70:D8:15" or "C0-C2-24-70-D8-15" to uint64.
func macStringToUint64(mac string) (uint64, error) {
	clean := strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", "")
	if len(clean) != 12 {
		return 0, fmt.Errorf("expected 12 hex digits, got %d", len(clean))
	}
	return strconv.ParseUint(clean, 16, 64)
}
