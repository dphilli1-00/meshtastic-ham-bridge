# meshtastic-ham-bridge

Bridges a Meshtastic mesh network to an HF/VHF ham radio link. Packets flow bidirectionally: Meshtastic nodes on one side, a Direwolf TNC, ARDOP modem, or audio device on the other.

```
[Meshtastic mesh] <-> [bridge] <-> [Direwolf / ARDOP / audio+rigctld]
```

Runs on Raspberry Pi (headless), Windows, Linux, and macOS. Supports serial and BLE connections to Meshtastic nodes.

---

## Status

### Implemented and tested
- **Core bridge loop** — bidirectional packet routing between MeshDevice and HamDevice
- **Meshtastic BLE adapter (Windows)** — connects by MAC address, receives FromRadio packets
- **Meshtastic serial adapter** — connects via USB serial, sends/receives protobuf
- **Meshcore adapter** — serial connection to Meshcore nodes
- **Direwolf KISS TCP adapter** — connects to a running Direwolf instance over TCP
- **BLE device discovery** — scans and lists nearby BLE devices with RSSI
- **Serial/audio device discovery** — lists ports and audio devices with Meshtastic hints
- **Test harness** — `--test-ble`, `--test-serial` CLI flags for end-to-end connection tests
- **Mock adapters** — MockMeshDevice and MockHamDevice for unit testing

### Implemented, not yet fully tested
- **Protobuf packet decoding** — FromRadio bytes are received but not yet unmarshalled
- **Config system** — TOML config loader is wired up but not exercised end-to-end
- **Raspberry Pi deployment** — Makefile targets and systemd unit exist, not yet field-tested

### Not yet implemented
- Packet forwarding (mesh → ham and ham → mesh) — adapters connect but don't bridge yet
- Graceful reconnect on connection loss
- PTT handling for audio/rigctld ham adapters
- CAT control (IC-705, hamlib)
- Web UI / remote monitoring
- Loopback test mode
- iOS audio adapter (PulseModem)
- Android/ChromeOS app shell
- Multi-hop routing and loop prevention
- Winlink gateway mode

---

## Requirements

- Go 1.22+
- A Meshtastic node (serial USB or BLE)
- One of: [Direwolf](https://github.com/wb2osz/direwolf), [ARDOP](https://www.cantab.net/users/john.wiseman/Documents/Latest%20Ardop%20Docs/ardopc.pdf), or a sound card modem (e.g. Digirig)

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
