//go:build windows

package mesh

// pairBLEDevice triggers Windows BLE pairing for a device that requires PIN entry.
// Windows won't show a pairing dialog unless you explicitly call the pairing API,
// so we use PowerShell to invoke the WinRT DeviceInformation pairing flow.
//
// The Meshtastic node will display a 6-digit PIN on its screen; the user enters
// it into the terminal here, and we feed it to the Windows pairing request.

import (
	"fmt"
	"os/exec"
	"strings"
)

// pairBLEDevice pairs a BLE device by MAC address using PowerShell WinRT calls.
// Prompts the user for the PIN displayed on the Meshtastic node screen.
// Safe to call if already paired — it checks first.
func pairBLEDevice(macAddr string) error {
	// Normalize MAC address to lowercase colon format
	mac := strings.ToLower(strings.ReplaceAll(macAddr, "-", ":"))

	// PowerShell script: check pairing status using WinRT via AsTask()
	// WindowsRuntimeSystemExtensions.AsTask() bridges WinRT IAsyncOperation to .NET Task
	psCheckPaired := fmt.Sprintf(`
$null = [Windows.Devices.Bluetooth.BluetoothLEDevice, Windows.Devices.Bluetooth, ContentType=WindowsRuntime]
$null = [Windows.Devices.Enumeration.DeviceInformationPairing, Windows.Devices.Enumeration, ContentType=WindowsRuntime]
Add-Type -AssemblyName System.Runtime.WindowsRuntime

function Await($op, $type) {
    $m = [System.WindowsRuntimeSystemExtensions].GetMethods() |
         Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 } |
         Select-Object -First 1
    $task = $m.MakeGenericMethod($type).Invoke($null, @($op))
    $task.Wait() | Out-Null
    $task.Result
}

$addrInt = [Convert]::ToUInt64(("%s" -replace ':', ''), 16)
$op = [Windows.Devices.Bluetooth.BluetoothLEDevice]::FromBluetoothAddressAsync($addrInt)
$device = Await $op ([Windows.Devices.Bluetooth.BluetoothLEDevice])
if ($null -eq $device) { Write-Output "NOTFOUND"; exit 1 }
if ($device.DeviceInformation.Pairing.IsPaired) { Write-Output "PAIRED" }
else { Write-Output "NOTPAIRED" }
`, mac)

	out, err := runPS(psCheckPaired)
	if err != nil {
		return fmt.Errorf("BLE pair check: %w", err)
	}
	out = strings.TrimSpace(out)

	if out == "PAIRED" {
		return nil // already paired, nothing to do
	}
	if out == "NOTFOUND" {
		return fmt.Errorf("BLE: device %s not found — is it in range?", macAddr)
	}

	fmt.Printf("BLE: pairing %s — watch for a Windows PIN dialog or notification...\n", macAddr)

	// Use non-custom PairAsync — Windows handles the PIN dialog via its own UI (toast/notification).
	// Custom pairing with PowerShell event handlers doesn't work for WinRT typed events.
	psPair := fmt.Sprintf(`
$null = [Windows.Devices.Bluetooth.BluetoothLEDevice, Windows.Devices.Bluetooth, ContentType=WindowsRuntime]
$null = [Windows.Devices.Enumeration.DeviceInformationPairing, Windows.Devices.Enumeration, ContentType=WindowsRuntime]
Add-Type -AssemblyName System.Runtime.WindowsRuntime

function Await($op, $type) {
    $m = [System.WindowsRuntimeSystemExtensions].GetMethods() |
         Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 } |
         Select-Object -First 1
    $task = $m.MakeGenericMethod($type).Invoke($null, @($op))
    $task.Wait() | Out-Null
    $task.Result
}

$addrInt = [Convert]::ToUInt64(("%s" -replace ':', ''), 16)
$op = [Windows.Devices.Bluetooth.BluetoothLEDevice]::FromBluetoothAddressAsync($addrInt)
$device = Await $op ([Windows.Devices.Bluetooth.BluetoothLEDevice])
if ($null -eq $device) { Write-Output "NOTFOUND"; exit 1 }

$pairOp = $device.DeviceInformation.Pairing.PairAsync()
$result = Await $pairOp ([Windows.Devices.Enumeration.DevicePairingResult])
Write-Output $result.Status
`, mac)

	out, err = runPS(psPair)
	if err != nil {
		return fmt.Errorf("BLE pairing: %w", err)
	}
	out = strings.TrimSpace(out)
	if out != "Paired" && out != "AlreadyPaired" {
		return fmt.Errorf("BLE pairing failed: %s", out)
	}
	fmt.Println("Paired successfully.")
	return nil
}

func runPS(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("powershell: %w\nstderr: %s", err, ee.Stderr)
		}
		return "", err
	}
	return string(out), nil
}
