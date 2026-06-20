import SwiftUI

struct ContentView: View {
    @StateObject private var bridge = BridgeManager()
    @State private var selectedDevice: String? = nil
    @State private var availableDevices: [String] = []

    var body: some View {
        NavigationView {
            Form {
                Section("Status") {
                    HStack {
                        Circle()
                            .fill(bridge.isRunning ? Color.green : Color.red)
                            .frame(width: 12, height: 12)
                        Text(bridge.statusMessage)
                    }
                }

                Section("Audio Device") {
                    if availableDevices.isEmpty {
                        Text("No audio devices found")
                            .foregroundColor(.secondary)
                    } else {
                        Picker("Device", selection: $selectedDevice) {
                            Text("System Default").tag(Optional<String>.none)
                            ForEach(availableDevices, id: \.self) { device in
                                Text(device).tag(Optional(device))
                            }
                        }
                    }
                    Button("Refresh Devices") {
                        availableDevices = bridge.listAudioDevices()
                    }
                }

                Section {
                    if bridge.isRunning {
                        Button("Stop Bridge", role: .destructive) {
                            bridge.stop()
                        }
                    } else {
                        Button("Start Bridge") {
                            bridge.start(audioDevice: selectedDevice)
                        }
                        .buttonStyle(.borderedProminent)
                    }
                }
            }
            .navigationTitle("Meshtastic Ham Bridge")
            .onAppear {
                availableDevices = bridge.listAudioDevices()
            }
        }
    }
}

#Preview {
    ContentView()
}
