import Foundation

/// Swift wrapper around the Rust bridge core.
///
/// Usage:
///   let manager = BridgeManager()
///   manager.start(audioDevice: "Digirig")  // or nil for default
///   manager.stop()
public class BridgeManager: ObservableObject {

    @Published public var isRunning = false
    @Published public var statusMessage = "Stopped"

    private var handle: OpaquePointer? = nil

    public init() {}

    deinit {
        stop()
    }

    // MARK: - Control

    /// Start the bridge. Audio device name is a substring match (e.g. "Digirig").
    /// Pass nil for the system default audio device.
    public func start(audioDevice: String? = nil) {
        guard handle == nil else {
            statusMessage = "Already running"
            return
        }

        let h: OpaquePointer?
        if let name = audioDevice {
            h = bridge_start_audio(name)
        } else {
            h = bridge_start_audio(nil)
        }

        if let h {
            handle = h
            isRunning = true
            statusMessage = "Running"
        } else {
            statusMessage = "Failed to start bridge"
        }
    }

    public func stop() {
        guard let h = handle else { return }
        bridge_stop(h)
        handle = nil
        isRunning = false
        statusMessage = "Stopped"
    }

    // MARK: - Device discovery

    /// Returns the list of available audio devices on this device.
    public func listAudioDevices() -> [String] {
        guard let raw = bridge_list_audio_devices() else { return [] }
        defer { bridge_free_string(raw) }
        return String(cString: raw)
            .split(separator: "\n")
            .map(String.init)
            .filter { !$0.isEmpty }
    }
}
