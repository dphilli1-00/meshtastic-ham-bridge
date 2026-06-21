# iOS App

Uses gomobile to generate a native iOS framework from the Go core.
No manual C FFI or bridging header needed.

## Hardware

Connect a USB audio adapter (e.g. Digirig) to the iPhone via the Apple USB-C Camera Adapter (CCK).
iOS supports USB Audio Class devices natively — no special driver needed.

PTT is handled via **VOX** (audio-triggered PTT configured on the radio itself). iOS does not
allow apps to access USB HID devices such as the CM108 GPIO used by Digirig for hardware PTT.
VOX works fine for packet data — AX.25 TXDelay accommodates the VOX tail with no impact on
throughput.

## Generate the framework (requires Mac + Xcode)

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
gomobile bind -target ios -o ios/MeshtasticHamBridge.xcframework \
  github.com/dphilli/meshtastic-ham-bridge/mobile
```

This produces `ios/MeshtasticHamBridge.xcframework` — a native Swift-importable framework.

## Xcode setup

1. Open Xcode → New Project → iOS App
2. Copy files from `ios/MeshtasticHamBridge/` into the project
3. Drag `MeshtasticHamBridge.xcframework` into the project
4. General → Frameworks, Libraries, Embedded Content → set to "Embed & Sign"
5. No bridging header needed — just `import Mobile` in Swift

## Info.plist permissions

```xml
<key>NSMicrophoneUsageDescription</key>
<string>Used to receive ham radio audio via AFSK modem.</string>
<key>NSBluetoothAlwaysUsageDescription</key>
<string>Used to connect to Meshtastic nodes via BLE.</string>
```

## Sideloading (no App Store needed)

1. Connect iPhone
2. Xcode → select your iPhone as target
3. Product → Run (signs with your free Apple ID)
Friends can sideload via AltStore or by building themselves.
