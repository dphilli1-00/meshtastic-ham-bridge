# iOS App

## Structure

```
ios/
  MeshtasticHamBridge/
    bridge.h                  C header for Rust FFI exports
    BridgingHeader.h          Xcode Objective-C bridging header
    BridgeManager.swift       Swift wrapper around the Rust core
    ContentView.swift         SwiftUI UI
    MeshtasticHamBridgeApp.swift  App entry point
```

## Build steps (requires Mac + Xcode)

### 1. Build the Rust static library for iOS

```bash
# Install iOS targets
rustup target add aarch64-apple-ios          # physical iPhone
rustup target add aarch64-apple-ios-sim      # simulator (Apple Silicon Mac)
rustup target add x86_64-apple-ios           # simulator (Intel Mac)

# Build
cargo build --target aarch64-apple-ios --release
cargo build --target aarch64-apple-ios-sim --release

# Output: target/aarch64-apple-ios/release/libmeshtastic_ham_bridge.a
```

### 2. Create Xcode project

1. Open Xcode → File → New → Project → iOS App
2. Product Name: `MeshtasticHamBridge`
3. Copy files from `ios/MeshtasticHamBridge/` into the project

### 3. Link the Rust library

In Xcode project settings:
- **General → Frameworks, Libraries, Embedded Content** → add `libmeshtastic_ham_bridge.a`
- **Build Settings → Library Search Paths** → add `$(PROJECT_DIR)/../../target/aarch64-apple-ios/release`
- **Build Settings → Swift Compiler - General → Objective-C Bridging Header** → `MeshtasticHamBridge/BridgingHeader.h`

### 4. Info.plist permissions

Add to `Info.plist`:
```xml
<key>NSMicrophoneUsageDescription</key>
<string>Meshtastic Ham Bridge uses the microphone to receive ham radio audio via AFSK.</string>
```

### 5. Run

Build and run on a physical iPhone or simulator.
The audio modem requires a real device for actual radio use.
