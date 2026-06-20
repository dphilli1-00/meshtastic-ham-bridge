# Session Context — Meshtastic HAM Bridge BLE Pairing

## Current Status
BLE pairing flow is ~95% working. Last blocker: Meshtastic device has a PIN configured,
requiring authenticated pairing. Our web pairing page now triggers service access to force
the PIN dialog, but this hasn't been tested yet.

## What Works
- Full BLE connection flow when device is already paired (50 packets received, clean)
- Web-based pairing page (`ble_pair_page.html`) opens Chrome automatically
- `IsPaired` check skips pairing on subsequent runs
- All WinRT COM GUIDs confirmed at runtime (no more guesses)
- `CoInitializeEx` scoped inside `pairBLEDevice` so `adapter.Enable()` works after

## Confirmed Real WinRT GUIDs (all verified at runtime via GetIids)
```
IDeviceInformationV1:          {ABA0FB95-4398-489D-8E44-E6130927011F}  ← from SDK, correct
IDeviceInformationStatics:     {C17F100E-3A46-4A78-8013-769DC9B97390}  ← from SDK, correct
IDeviceInformation2:           {F156A638-7997-48D9-A10C-269D46533F48}  ← was wrong, fixed
IDeviceInformationPairing:     {2C4769F5-F684-40D5-8469-E8DBAAB70485}  ← was wrong, fixed
IDeviceInformationPairing2:    {F68612FD-0AEE-4328-85CC-1C742BB1790D}  ← was wrong, fixed
IDevicePairingResult:          {072B02BF-DD95-4025-9B37-DE51ADBA37B7}  ← was wrong, fixed
iBluetoothLEDevice2 (for QI):  {26F062B3-7AEE-4D31-BABA-B1B9775F5916}  ← winrt-go value
```

## Key Files
- `internal/mesh/ble_pair_windows.go` — main pairing flow
- `internal/mesh/ble_pair_web_windows.go` — HTTP server + Chrome launcher for web pairing
- `internal/mesh/ble_pair_page.html` — embedded pairing UI (Web Bluetooth)
- `internal/mesh/winrt_enumeration_windows.go` — hand-written WinRT COM bindings
- `internal/mesh/ble_pair_other.go` — stub for non-Windows builds
- `internal/mesh/meshtastic_ble.go` — calls `pairBLEDevice` then connects via tinygo

## Current Pairing Flow
1. `pairBLEDevice(macAddr)` called from `ConnectMeshtasticBLE`
2. Scoped `CoInitializeEx` / `defer CoUninitialize`
3. `BluetoothLEDeviceFromBluetoothAddressAsync` → await → BluetoothLEDevice
4. QI for `iBluetoothLEDevice2` → `getDeviceInformation()` → partial DeviceInfo
5. QI partial for `IDeviceInformationV1` → `getId()` → device ID string
6. `DeviceInformation.CreateFromIdAsync(deviceId)` → full DeviceInfo
7. Probe loop: try `{F156A638}` and `{2743208B}` for IDeviceInformation2, validate by
   checking slot 7 result's runtime class name == DeviceInformationPairing
8. `pairing.getIsPaired()` → if true, return nil (fast path)
9. If not paired: `webPairBLE(macAddr)` → opens Chrome to local HTTP page
10. Page uses `acceptAllDevices:true` + `optionalServices:[MESHTASTIC_SERVICE]`
11. After user selects device: `gatt.connect()` → `getPrimaryService()` → forces PIN dialog
12. Page POSTs `/done` after 2s settle delay → Go returns
13. Go sleeps 1s more, returns nil
14. `CoUninitialize()` runs, then `adapter.Enable()` + tinygo connects

## Last Thing Tested
Web pairing page opened, device paired WITHOUT triggering PIN dialog → subsequent
BLE reads failed ("The parameter is incorrect" / async status 3). Root cause: unauthenticated
bond. Fix applied: page now calls `getPrimaryService()` + `getCharacteristic()` after
`gatt.connect()` to force authenticated pairing + PIN prompt. NOT YET TESTED.

## Test Devices
- `C0:C2:24:70:D8:15` — "silver" — has a stale bond problem (bonded to old Chrome key,
  Meshtastic side still has it). Avoid until factory reset.
- `F8:5A:43:72:45:62` — "Meshtastic_4562" — use this for testing. Has PIN configured.

## Test Command
```
go run ./cmd/bridge --test-ble F8:5A:43:72:45:62
```
Expected: browser opens → device picker → select Meshtastic_4562 → PIN dialog appears →
enter PIN from device screen → page shows "Paired" → Go connects → packets received.

## Pending Cleanup (after pairing works)
- Strip all diagnostic `fmt.Printf` from `ble_pair_windows.go` and `winrt_enumeration_windows.go`
  (probe loop prints, IID dumps, characteristic prints, poll tick counter)
- Update comment block at top of `ble_pair_windows.go`
- `iDeviceInformationPairing2`, `iDeviceInformationCustomPairing`,
  `iDevicePairingRequestedEventArgs` structs in `winrt_enumeration_windows.go` are now dead
  code (kept for reference) — can be pruned

## Build
```
go build ./cmd/bridge      # Windows only for BLE
go build ./...             # needs GOOS=windows or run on Windows
```
No new dependencies added this session.
