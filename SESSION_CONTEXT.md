# Session Context — Meshtastic HAM Bridge

## Project Goal
Bridge a Meshtastic LoRa mesh network to an HF/VHF ham radio link. Packets flow
bidirectionally between Meshtastic nodes and a Direwolf TNC, AFSK audio modem, or
rigctld-controlled radio. Runs on Raspberry Pi (headless) or Windows/Linux/macOS.
Target hardware: small sealed go-box with Pi + Meshtastic node + Digirig/audio interface.

No equivalent open-source project exists. Hamtastic (JS8Call/Node-RED) is the closest
but requires internet and is heavyweight. This project is local-RF-only, single Go binary,
no runtime dependencies.

---

## Implementation Status

### Implemented and tested
- Core bridge loop (`internal/bridge/bridge.go`) — bidirectional routing, unit-tested
- Mock adapters (`internal/ham/mock.go`, `internal/mesh/mock.go`) — used in bridge tests
- Meshtastic BLE adapter Windows (`internal/mesh/meshtastic_ble.go` + `ble_pair_windows.go`)
  — connects by MAC, discovers GATT, receives raw FromRadio bytes; confirmed working
- Meshtastic serial adapter (`internal/mesh/meshtastic.go`) — serial transport works
- BLE device discovery (`internal/discovery/discovery_ble.go`)
- Serial/audio device discovery (`internal/discovery/discovery.go`)
- rigctld adapter (`internal/ham/rigctl.go`) — unit-tested with fake server
- Test harness — `--test-ble`, `--test-serial` CLI flags

### Implemented with known gaps
- Direwolf KISS TCP (`internal/ham/direwolf.go`) — full KISS framing, **RF loopback tested and passing**
- Bell 202 AFSK audio modem (`internal/ham/audio.go`) — full TX/RX implementation,
  not tested on real hardware
- Meshtastic send (serial + BLE) — transport works but no ToRadio protobuf wrapping yet
- Meshcore serial adapter (`internal/mesh/meshcore.go`) — transport only, no protocol framing
- Config loader (`internal/config/config.go`) — TOML works; --init-config missing,
  config.Template() undefined (compilation error if invoked)
- Mobile bridge (`mobile/mobile.go`) — BLE path and audio+rigctl not wired

### Not yet implemented
- Protobuf decoding of received FromRadio packets (task #25)
- ToRadio protobuf wrapping for outgoing packets
- Actual packet forwarding (adapters connect but no packets flow through bridge yet)
- Meshcore protocol framing
- Graceful reconnect on connection loss
- PTT sequencing for audio/rigctld
- CAT control (IC-705, hamlib)
- --init-config / config template
- iOS audio adapter (PulseModem / Swift FFI)
- Android/ChromeOS app shell
- Web UI and remote monitoring
- Multi-hop routing and loop prevention
- Winlink gateway mode
- Pi deployment (Makefile targets exist, not field-tested)

---

## BLE Pairing (Windows) — Resolved
- Check IsPaired via WinRT COM; if already paired, connect directly (works perfectly)
- If not paired: return clear error telling user to pair via Meshtastic app first
- WinRT custom pairing with TypedEventHandler was attempted and abandoned —
  Go cannot safely implement COM delegate callbacks against the WinRT ABI
- Web Bluetooth pairing page approach was also tried but bonds without auth (PIN not triggered)
- Confirmed working: pair once via Meshtastic app → bridge connects automatically forever

## Confirmed WinRT GUIDs (runtime-verified)
```
IDeviceInformationV1:            {ABA0FB95-4398-489D-8E44-E6130927011F}
IDeviceInformationStatics:       {C17F100E-3A46-4A78-8013-769DC9B97390}
IDeviceInformation2:             {F156A638-7997-48D9-A10C-269D46533F48}
IDeviceInformationPairing:       {2C4769F5-F684-40D5-8469-E8DBAAB70485}
IDeviceInformationPairing2:      {F68612FD-0AEE-4328-85CC-1C742BB1790D}
IDeviceInformationCustomPairing: {85138C02-4EE6-4914-8370-107A39144C0E}
IDevicePairingResult:            {072B02BF-DD95-4025-9B37-DE51ADBA37B7}
iBluetoothLEDevice2:             {26F062B3-7AEE-4D31-BABA-B1B9775F5916}
```

---

## Key Files
```
cmd/bridge/main.go                  CLI entry point
internal/bridge/bridge.go           Core bridge loop
internal/config/config.go           TOML config
internal/platform/platform.go       Single source of truth for OS/capabilities/binary paths
internal/discovery/                 Serial, BLE, audio discovery
internal/ham/
  direwolf.go                       KISS TCP adapter + DirewolfDevice struct (cmd field for subprocess)
  direwolf_launch.go                LaunchDirewolf / KillAllDirewolf / portInUse
  direwolf_config.go                Interactive setup wizard (RunDirewolfSetup)
  direwolf_config_parse.go          ReadKISSPort — parses KISSPORT from config file
  direwolf_devices.go               DirewolfAudioDevice struct (probing abandoned, use malgo directly)
  audio.go                          Bell 202 AFSK modem + AudioDeviceInfo + ListAudioInputs/OutputsIndexed
  rigctl.go                         rigctld PTT/CAT control
  mock.go                           Test mock
internal/mesh/
  meshtastic.go                     Serial adapter
  meshtastic_ble.go                 BLE adapter (all platforms)
  ble_pair_windows.go               Windows BLE pairing (WinRT COM)
  ble_pair_web_windows.go           Web pairing page server (unused, kept)
  ble_pair_page.html                Embedded pairing UI (unused, kept)
  ble_pair_other.go                 Non-Windows stub
  winrt_enumeration_windows.go      Hand-written WinRT COM bindings
  meshcore.go                       Meshcore serial (transport only)
  mock.go                           Test mock
mobile/mobile.go                    gomobile bridge (partial)
direwolf-tx.conf                    Working TX config (digirig tx, KISSPORT 8001, CM108 PTT, AGWPORT 0)
direwolf-rx.conf                    Working RX config (digirig rx, KISSPORT 8002, CM108 PTT, AGWPORT 0)
```

---

## Test Devices
- `F8:5A:43:72:45:62` — "Meshtastic_4562" — primary test device, working
- `C0:C2:24:70:D8:15` — "silver" — has stale bond issue, avoid for now

## Build & Test
```powershell
go build ./cmd/bridge
go run ./cmd/bridge --test-ble F8:5A:43:72:45:62
go run ./cmd/bridge --discover-ble-all
go test ./...

# Direwolf RF loopback (order of configs doesn't matter — lower KISSPORT always launches first)
taskkill /F /IM direwolf.exe
go run ./cmd/bridge --test-direwolf direwolf-tx.conf,direwolf-rx.conf

# Single-radio TX loop or RX listen
go run ./cmd/bridge --test-direwolf direwolf-tx.conf
go run ./cmd/bridge --test-direwolf direwolf-rx.conf --rx

# Interactive config wizard
go run ./cmd/bridge --setup-direwolf direwolf-tx.conf
go run ./cmd/bridge --setup-direwolf direwolf-rx.conf
```

## Direwolf / KISS Notes
- Direwolf 1.8.1 always binds port 8001 as a default KISS port in addition to any configured KISSPORT.
  AGWPORT 0 disables the AGW port but does NOT suppress the extra 8001 KISS bind.
  Fix: always launch the lower-KISSPORT instance first (done in main.go two-config path).
- PTT: CM108 HID paths are device-specific. Run `cm108` utility to find yours. Include explicit
  HID path in config — do not rely on Direwolf auto-detection when multiple CM108 devices present.
- Audio device names: rename in Windows Sound control panel to disambiguate identical "USB Audio Device"
  names. Use short names without suffix in ADEVICE (e.g. "digirig tx mic" not "digirig tx mic (USB Audio Device)").
- ADEVICE order: input (receive) first, then output (transmit): `ADEVICE "input" "output"`
- CGO required for malgo audio device enumeration. On Windows: install TDM-GCC, set CGO_ENABLED=1.
  If signed binary is blocked by Windows, use `go run` instead of the built binary.

## Direwolf Config Template (working)
```
MYCALL KC1SMQ
ADEVICE "digirig tx mic" "digirig tx speaker"
ACHANNELS 1
CHANNEL 0
MODEM 1200
PTT CM108 "\\?\hid#vid_0d8c&pid_0012&mi_03#7&264aaa23&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}"
KISSPORT 8001
AGWPORT 0
```

---

## Immediate Next Steps (priority order)
1. Decode received FromRadio protobuf bytes (import meshtastic-go protobufs)
2. Handle config stream: drain MyNodeInfo + NodeInfo + ConfigCompleteId before "ready"
3. Wrap outgoing packets in ToRadio protobuf envelope
4. Wire up actual packet forwarding through the bridge loop
5. Wire up buildHam() in main.go to use LaunchDirewolfWithBinary with platform detection
6. Strip diagnostic fmt.Printf calls from ble_pair_windows.go and meshtastic_ble.go
7. Fix --init-config (config.Template() missing)

## Audio Hardware Notes
- Digirig = USB audio (CM108) + PTT GPIO in one plug. Convenient but not required.
- Any CM108-based USB audio dongle (~$5) is functionally equivalent.
- Many modern laptops and phones have no audio jack — solution is the same everywhere:
    USB-C → USB audio adapter (Digirig or equivalent) → Radio
- This is a standard USB Audio Class device. Works on Windows, Linux, macOS, Android, iOS (via CCK).
- Phone IS the radio interface. No Pi required for mobile use — phone audio + modem + Meshtastic BLE.
- ConnectDirewolf (remote) vs LaunchDirewolfWithBinary (local) — distinction is whether cmd is nil.
- Config wizard should detect available audio output devices and warn / steer toward remote Direwolf if none found.

## PTT Strategy (per platform)
- **Desktop (Windows/Linux/macOS)**: CM108 GPIO via Direwolf, or RTS/DTR via USB serial, or rigctld CAT
- **Android**: CM108 GPIO via UsbManager HID, or VOX
- **iOS**: VOX only — iOS blocks USB HID access to CM108. This is fine: this app sends packets,
  not real-time voice. AX.25 already has TXDelay; a few hundred ms of VOX tail doesn't matter.
- **VOX** is the universal fallback and works everywhere with zero extra code.
  Enable on the radio side; no PTT logic needed in software.

## SDR Discussion (this session)
Considered using SDR to replace all RF hardware with one device. Conclusion: not practical.
- LoRa (Meshtastic) has no reliable SDR TX implementation
- CPU cost too high for Pi Zero
- Dedicated hardware ($30 Meshtastic node + $40 Digirig) beats SDR on cost, power, simplicity
- SDR useful only as RX monitor at a base station with spare CPU
