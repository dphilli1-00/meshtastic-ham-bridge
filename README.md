# meshtastic-ham-bridge

Bridges a Meshtastic mesh network to an HF/VHF/UHF ham radio link. Packets flow bidirectionally: Meshtastic nodes on one side, a Direwolf TNC or OFDM audio modem on the other.

```
[Meshtastic mesh] <-> [bridge] <-> [Direwolf / OFDM modem] <-> [Radio]
```

Runs on Raspberry Pi (headless), Windows, Linux, macOS, Android, and iOS. Supports serial and BLE connections to Meshtastic nodes. No internet required — local RF only, single Go binary, no runtime dependencies.

---

## Hardware

The radio interface is a standard USB audio adapter (e.g. [Digirig](https://digirig.net)) connected between the device and the radio. This works on any platform — including phones and tablets that lack a headphone jack — via a USB-C adapter.

```
[Phone / Laptop / Pi]
        |
   USB-C / USB-A
        |
  [USB audio adapter]  ← Digirig or any CM108-based USB audio dongle
        |
    3.5mm cable
        |
    [Ham radio]        ← VHF/UHF FM or HF SSB
```

The modem operates in audio space (300–3000 Hz). The radio converts audio to RF. The same software modem works on HF SSB, VHF FM, and UHF FM — only the radio changes.

### PTT (keying the transmitter)

| Platform | PTT method |
|----------|------------|
| Desktop / Pi | CM108 GPIO via Digirig (working), or RTS/DTR serial, or CAT via rigctld |
| Android | CM108 GPIO via UsbManager HID |
| iOS | VOX (audio-triggered PTT on the radio). Fine for packet — not voice. AX.25 TXDelay covers the VOX tail. |

**Cable note:** Green Baofeng K1 cables have unreliable PTT contact at the 2.5mm plug. Use black K1 cables.

### Tested hardware

- Digirig (CM108 USB audio + PTT GPIO in one plug)
- Baofeng UV-5R (VHF/UHF FM)
- Icom IC-705 (HF/VHF/UHF, USB audio built in — no Digirig needed)
- Meshtastic nodes over serial USB and BLE

---

## Modem

Two RF paths are supported:

### Direwolf (default — desktop and Pi)

AX.25 / Bell 202 AFSK 1200 baud. Interoperable with any APRS TNC. Direwolf runs as a subprocess; the bridge connects via KISS TCP.

```sh
go build ./cmd/bridge          # uses Direwolf
```

Direwolf must be installed separately. See [wb2osz/direwolf](https://github.com/wb2osz/direwolf).

**RF loopback tested and working** with two Baofengs + two Digirigs on Windows.

### liquid-dsp OFDM (iOS / Android / Pi — `-tags liquid`)

OFDM flex frame modem using [liquid-dsp](https://github.com/jgaeddert/liquid-dsp). No Direwolf required. Designed for mobile and embedded platforms.

Why OFDM: pilot-symbol sync, built-in FEC and CRC, handles HF multipath and FM pre-emphasis automatically. Works across HF SSB, VHF FM, and UHF FM with the same audio bandwidth.

```sh
go build -tags liquid ./cmd/bridge    # uses liquid-dsp OFDM
```

**Platform support for `-tags liquid`:**

| Platform | Status |
|----------|--------|
| Linux / Pi | Works — `apt install libliquid-dev` |
| macOS | Works — `brew install liquid-dsp` |
| iOS | Works — Xcode toolchain, CGO + gomobile |
| Android | Works — NDK CMake toolchain |
| Windows | **Not supported** — liquid-dsp 2.x uses Newlib symbols (`__getreent`, `strsep`) incompatible with MinGW libc. Use Direwolf on Windows. |

---

## Status

### Implemented and tested
- **Core bridge loop** — bidirectional packet routing, unit-tested
- **Meshtastic BLE adapter (Windows)** — connects by MAC, GATT discovery, receives raw FromRadio bytes; confirmed working
- **Meshtastic serial adapter** — serial transport, background read loop
- **BLE / serial / audio device discovery**
- **rigctld adapter** — PTT + CAT control, unit-tested with fake server
- **Direwolf KISS TCP adapter** — full KISS framing, subprocess launch; **RF loopback tested end-to-end** with two radios and two Digirigs
- **Direwolf config wizard** — interactive setup, audio device enumeration
- **CM108 PTT** — GPIO4 via HID, working on Windows with Digirig; auto-detects by VID/PID
- **liquid-dsp OFDM modem** — CGO wrapper, TX/RX IQ pipeline, malgo audio I/O; compiles on Linux/macOS/iOS/Android

### Implemented with known gaps
- **Bell 202 AFSK modem (Go-native)** — TX tones confirmed clean on radio; RX discriminator works but clock recovery (Gardner TED) doesn't lock. Parked — liquid-dsp replaces this on mobile.
- **Meshtastic send** — transport works; no ToRadio protobuf wrapping yet
- **Meshcore serial adapter** — transport only; protocol framing not implemented
- **Config loader** — TOML works; `--init-config` and config template not yet implemented
- **Mobile bridge (gomobile)** — BLE and audio paths not fully wired

### Not yet implemented
- Protobuf decoding of received `FromRadio` packets (next major milestone)
- `ToRadio` protobuf wrapping for outgoing packets
- Actual packet forwarding (adapters connect but no packets flow through bridge yet)
- Meshcore protocol framing
- Graceful reconnect on connection loss
- CAT control (IC-705, generic hamlib)
- `--init-config` / config template
- Android / iOS app shell (gomobile)
- Web UI and remote monitoring
- Multi-hop routing and loop prevention
- Winlink gateway mode
- Pi deployment (Makefile targets exist, not field-tested)

---

## Requirements

- Go 1.22+ with CGO enabled
  - Windows: TDM-GCC (for non-liquid builds) or MSYS2 MinGW64 (for liquid builds)
  - Linux/Pi: gcc + libliquid-dev (for liquid builds)
- A Meshtastic node (serial USB or BLE)
- A USB audio adapter connecting your device to the radio (Digirig recommended)
- [Direwolf](https://github.com/wb2osz/direwolf) for desktop/Pi use without `-tags liquid`

---

## Build

```sh
# Standard build (Direwolf path)
go build -o meshtastic-ham-bridge ./cmd/bridge

# liquid-dsp OFDM modem (Linux/Pi/Mac/iOS/Android)
go build -tags liquid -o meshtastic-ham-bridge ./cmd/bridge
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

Config locations:

- **Windows:** `%APPDATA%\meshtastic-ham-bridge\config.toml`
- **macOS:** `~/Library/Application Support/meshtastic-ham-bridge/config.toml`
- **Linux/Pi:** `~/.config/meshtastic-ham-bridge/config.toml`

Run:

```sh
meshtastic-ham-bridge
meshtastic-ham-bridge --config /path/to/config.toml
```

---

## Configuration

Config is TOML. Multiple bridges can run simultaneously.

### Meshtastic over serial → Direwolf

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

### Meshtastic over BLE → audio modem

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
name = "ic705-hf"

[bridge.mesh]
type = "meshtastic"

[bridge.ham]
type         = "audio+rigctl"
audio_device = "IC-705"
rigctl_host  = "127.0.0.1"
rigctl_port  = 4532
```

```sh
rigctld -m 3085 -r /dev/ttyUSB0 -s 19200
```

### Config fields

**Mesh:**

| Field | Default | Description |
|-------|---------|-------------|
| `type` | — | `meshtastic` or `meshcore` |
| `port` | auto | Serial port (`COM4`, `/dev/ttyUSB0`); omit to auto-discover |
| `ble_address` | — | BLE MAC address; takes priority over serial |
| `baud_rate` | `115200` | Serial baud rate |

**Ham:**

| Field | Default | Description |
|-------|---------|-------------|
| `type` | — | `direwolf`, `audio`, or `audio+rigctl` |
| `host` | `127.0.0.1` | Direwolf host |
| `port` | `8001` | Direwolf KISS TCP port |
| `audio_device` | default | Substring match against OS audio device names |
| `rigctl_host` | `127.0.0.1` | rigctld host |
| `rigctl_port` | `4532` | rigctld port |

---

## Discovery and Diagnostics

```sh
# List serial ports, BLE devices, audio devices
meshtastic-ham-bridge --discover

# All BLE devices (including non-Meshtastic)
meshtastic-ham-bridge --discover-ble-all

# List CM108 HID devices (find PTT path)
meshtastic-ham-bridge --list-cm108

# List audio devices
meshtastic-ham-bridge --list-audio

# Test serial connection (sends WantConfigId, reads 10s)
meshtastic-ham-bridge --test-serial COM4
meshtastic-ham-bridge --test-serial /dev/ttyUSB0

# Test BLE connection
meshtastic-ham-bridge --test-ble C0:C2:24:70:D8:15

# Direwolf RF loopback (two radios)
meshtastic-ham-bridge --test-direwolf direwolf-tx.conf,direwolf-rx.conf

# Direwolf single-radio TX or RX
meshtastic-ham-bridge --test-direwolf direwolf-tx.conf
meshtastic-ham-bridge --test-direwolf direwolf-rx.conf --rx

# Audio modem loopback
meshtastic-ham-bridge --test-audio "digirig tx","digirig rx"

# Raw tone test (verify modem and PTT without framing)
meshtastic-ham-bridge --test-audio-tones "digirig tx","digirig rx"
```

---

## Direwolf Setup

Interactive config wizard (sets up audio devices, PTT, KISS port):

```sh
meshtastic-ham-bridge --setup-direwolf direwolf-tx.conf
meshtastic-ham-bridge --setup-direwolf direwolf-rx.conf
```

**Direwolf quirks:**
- Direwolf 1.8.1 always binds port 8001 as a default KISS port even if you configure a different port. Always launch the lower-port instance first.
- When multiple CM108 devices are present, specify the full HID path explicitly in the config. Run `--list-cm108` to find it.
- Audio device names: if you have multiple "USB Audio Device" entries, rename them in Windows Sound control panel.
- HID paths are not stable across USB replugs on Windows. If PTT stops working, run `--list-cm108` and update your config.

---

## Windows BLE Notes

Windows requires a device to be bonded before GATT reads/writes succeed:

1. Open Chrome → [client.meshtastic.org](https://client.meshtastic.org)
2. Connect to your node via BLE (establishes the bond)
3. Disconnect from the browser
4. Run the bridge — it reuses the existing bond

The bridge connects by MAC address without scanning, so it works even if the node is already connected to another host.

---

## Raspberry Pi Deployment

```sh
make deploy PI=pi@raspberrypi.local
make status PI=pi@raspberrypi.local
```

Builds for arm64, copies the binary, installs the systemd unit, and starts the service.

```sh
sudo systemctl status meshtastic-ham-bridge
sudo journalctl -u meshtastic-ham-bridge -f
```

---

## Project Layout

```
cmd/bridge/                 entry point + CLI flags
internal/
  bridge/                   packet routing loop
  config/                   TOML config loader
  discovery/                serial + BLE + audio device discovery
  ham/
    direwolf.go             Direwolf KISS TCP adapter
    audio.go                Bell 202 AFSK modem (Go-native, parked)
    liquid/                 liquid-dsp OFDM modem (CGO, -tags liquid)
    ptt.go                  PTT interface
    ptt_cm108.go            CM108 HID PTT (desktop/Pi)
    rigctl.go               rigctld PTT/CAT adapter
  mesh/
    meshtastic.go           Meshtastic serial adapter
    meshtastic_ble.go       Meshtastic BLE adapter
    meshcore.go             Meshcore serial adapter
  types/                    shared Packet and DeviceStatus types
mobile/                     gomobile bridge entry point (partial)
direwolf-tx.conf            working TX Direwolf config
direwolf-rx.conf            working RX Direwolf config
SESSION_CONTEXT.md          internal dev notes and decisions
```

---

## License

MIT
