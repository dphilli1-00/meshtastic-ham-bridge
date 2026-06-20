// This file is no longer used.
// The iOS app now uses the gomobile-generated MeshtasticHamBridge.xcframework
// instead of a manual C FFI layer.
//
// To regenerate the framework:
//   go install golang.org/x/mobile/cmd/gomobile@latest
//   gomobile init
//   gomobile bind -target ios -o ios/MeshtasticHamBridge.xcframework \
//     github.com/dphilli/meshtastic-ham-bridge/mobile
//
// Then in Xcode: remove this file and add MeshtasticHamBridge.xcframework instead.
