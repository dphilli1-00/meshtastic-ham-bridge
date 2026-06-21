# meshtastic-ham-bridge

Bridges a Meshtastic mesh network to an HF/VHF ham radio link. Packets flow bidirectionally: Meshtastic nodes on one side, a Direwolf TNC or audio modem on the other.

```
[Meshtastic mesh] <-> [bridge] <-> [Direwolf / audio modem] <-> [Radio]
```

Runs on Raspberry Pi (headless), Windows, Linux, macOS, Android, and iOS. Supports serial and BLE connections to Meshtastic nodes.

### Hardware

The radio interface is a standard USB audio adapter (e.g. [Digirig](https://digirig.net)) connected between the device and the radio. This works on any platform — including phones and tablets that lack a headphone jack — via a USB-C adapter.

```
[Phone / Laptop / Pi]
        |
   USB-C / USB-A
        |
  [USB audio adapter]  ← Digirig or any USB audio dongle
        |
    3.5mm cable
        |
    [Ham radio]
```

PTT (keying the transmitter) is handled per platform:
- **Desktop / Android** — CM108 GPIO via the Digirig, or RTS/DTR via USB serial, or CAT via rigctld
- **iOS** — VOX (audio-triggered PTT on the radio). This is fine: the bridge sends data packets, not real-time voice. AX.25 TXDelay already accommodates the VOX tail.

---

## Status

### Implemented and tested
- **Core bridge loop** — bidirectional packet routing between MeshDevice and HamDevice, with unit tests
- **Mock adapters** — MockMeshDevice and MockHamDevice used in bridge tests
- **Meshtastic BLE adapter (Windows)** — connects by MAC address, discovers GATT characteristics, receives raw FromRadio bytes
- **Meshtastic serial adapter** — opens serial port, background read loop, channel-based receive
- **BLE device discovery** — scans and lists nearby BLE devices with RSSI
- **Serial / audio device discovery** — lists ports and audio devices with Meshtastic/Digirig hints
- **rigctld adapter** — TCP connection to rigctld, PTT keying, frequency/mode control; unit-tested with a fake server
- **Direwolf KISS TCP adapter** — full KISS framing, subprocess launch, CM108 PTT; RF loopback tested end-to-end with two radios and two Digirigs
- **Direwolf config wizard** — interactive setup (`--setup-direwolf`), audio device enumeration via malgo
- **RF loopback test** — `--test-direwolf tx.conf,rx.conf`; config file order doesn't matter
- **Test harness** — `--test-ble`, `--test-serial`, `--test-direwolf` CLI flags

### Implemented with known gaps
- **Bell 202 AFSK audio modem** — HDLC encode/bit-stuffing on TX, correlator demodulator + HDLC framing on RX via miniaudio; implemented but not tested on real hardware
- **Meshtastic send (serial + BLE)** — transport works but outgoing packets are not yet wrapped in the `ToRadio` protobuf envelope; sending does not produce valid output
- **Meshcore serial adapter** — transport layer only; protocol framing not implemented
- **Config loader** — TOML round-trip works; `--init-config` and the config template function are not yet implemented
- **Mobile bridge (gomobile)** — BLE path and audio ham type not wired; audio discovery not implemented

### Not yet implemented
- Protobuf decoding of received `FromRadio` packets
- `ToRadio` protobuf wrapping for outgoing packets
- Actual packet forwarding between mesh and ham sides (adapters connect independently; no packets flow through yet)
- Meshcore protocol framing
- Graceful reconnect on connection loss
- CAT control (IC-705, generic hamlib)
- `--init-config` / config template
- Android / iOS / ChromeOS app shell
- Web UI and remote monitoring
- Multi-hop routing and loop prevention
- Winlink gateway mode
- Raspberry Pi deployment (Makefile targets exist, not field-tested)

---

## Requirements

- Go 1.22+ (with CGO enabled for audio device enumeration — requires TDM-GCC on Windows)
- A Meshtastic node (serial USB or BLE)
- A USB audio adapter connecting your device to the radio (e.g. [Digirig](https://digirig.net))
- [Direwolf](https://github.com/wb2osz/direwolf) for desktop/Pi use (not required on mobile)

---

## Build

```sh
go build -o meshtastic-ham-bridge ./cmd/bridge
```

Cross-compile for Raspberry Pi:

```sh
make build-pi      # Pi 4/5 (arm64)
make build-pi32    # Pi 3 and older (arm)
```

---

## Quick Start

Generate a starter config:

```sh
meshtastic-ham-bridge --init-config
```

The config is written to the platform default location:

- **Windows:** `%APPDATA%\meshtastic-ham-bridge\config.toml`
- **macOS:** `~/Library/Application Support/meshtastic-ham-bridge/config.toml`
- **Linux/Pi:** `~/.config/meshtastic-ham-bridge/config.toml`

Run:

```sh
meshtastic-ham-bridge
```

Or point at a specific config:

```sh
meshtastic-ham-bridge --config /path/to/config.toml
```

---

## Configuration

Config is TOML. Multiple bridges can run simultaneously.

### Meshtastic over serial (auto-discover)

```toml
[[bridge]]
name = "vhf-local"

[bridge.mesh]
type = "meshtastic"
# port = "COM4"     # omit to auto-discover

[bridge.ham]
type = "direwolf"
host = "127.0.0.1"
port = 8001
```

### Meshtastic over BLE

```toml
[[bridge]]
name = "ble-to-hf"

[bridge.mesh]
type        = "meshtastic"
ble_address = "C0:C2:24:70:D8:15"   # from --discover output

[bridge.ham]
type         = "audio"
audio_device = "Digirig"             # substring match on device name
```

### IC-705 with rigctld PTT

```toml
[[bridge]]
name = "ic705-vhf"

[bridge.mesh]
type = "meshtastic"

[bridge.ham]
type         = "audio+rigctl"
audio_device = "IC-705"
rigctl_host  = "127.0.0.1"
rigctl_port  = 4532
```

Start rigctld first:

```sh
rigctld -m 3085 -r /dev/ttyUSB0 -s 19200
```

### Mesh config fields

| Field | Default | Description |
|-------|---------|-------------|
| `type` | — | `meshtastic` or `meshcore` |
| `port` | auto | Serial port (e.g. `COM4`, `/dev/ttyUSB0`); omit to auto-discover |
| `ble_address` | — | BLE MAC address; takes priority over serial |
| `baud_rate` | `115200` | Serial baud rate |

### Ham config fields

| Field | Default | Description |
|-------|---------|-------------|
| `type` | — | `direwolf`, `ardop`, `audio`, or `audio+rigctl` |
| `host` | `127.0.0.1` | Direwolf / ARDOP host |
| `port` | `8001` / `8515` | Direwolf (8001) or ARDOP (8515) port |
| `audio_device` | default | Substring match against OS audio device names |
| `rigctl_host` | `127.0.0.1` | rigctld host |
| `rigctl_port` | `4532` | rigctld port |

---

## Discovery and Diagnostics

Find what's plugged in:

```sh
meshtastic-ham-bridge --discover
```

Output shows serial ports, nearby BLE devices, and audio devices with hints for Meshtastic nodes and Digirig interfaces.

Show all BLE devices (including non-Meshtastic):

```sh
meshtastic-ham-bridge --discover-ble-all
```

Test a serial connection end-to-end (sends WantConfigId, reads 10s):

```sh
meshtastic-ham-bridge --test-serial COM4
meshtastic-ham-bridge --test-serial /dev/ttyUSB0
```

Test a BLE connection:

```sh
meshtastic-ham-bridge --test-ble C0:C2:24:70:D8:15
```

---

## Windows BLE Notes

Windows requires a device to be bonded before GATT reads/writes succeed. The easiest way to pre-bond a Meshtastic node:

1. Open Chrome and go to [client.meshtastic.org](https://client.meshtastic.org)
2. Connect to your node via BLE — Chrome's WebBluetooth establishes the bond
3. Disconnect from the browser
4. Run the bridge — it reuses the existing bond

The bridge connects directly by MAC address without scanning, so it works even when the node is already connected to another host.

---

## Raspberry Pi Deployment

```sh
# Build and deploy in one step
make deploy PI=pi@raspberrypi.local

# Check status
make status PI=pi@raspberrypi.local
```

This builds for arm64, copies the binary, installs the systemd unit, and starts the service. The service runs as the `pi` user with access to `dialout` (serial) and `audio` groups.

Manual service management:

```sh
sudo systemctl status meshtastic-ham-bridge
sudo journalctl -u meshtastic-ham-bridge -f
```

---

## Project Layout

```
cmd/bridge/          entry point
internal/
  bridge/            packet routing loop
  config/            TOML config loader
  discovery/         serial + BLE + audio device discovery
  ham/               ham radio backends (Direwolf, ARDOP, audio, rigctld)
  mesh/              Meshtastic and Meshcore backends (serial + BLE)
  types/             shared types
configs/             example config files
deploy/systemd/      systemd unit file
scripts/             Pi install helper
```

---

## License

MIT
