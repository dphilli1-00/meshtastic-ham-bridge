#pragma once
#include <stdint.h>

/// Opaque handle to a running bridge instance.
typedef struct BridgeHandle BridgeHandle;

/// Start the bridge using the audio modem (AFSK Bell 202) for ham radio.
/// device_name: NULL = system default audio device, otherwise substring match.
/// Returns NULL on failure.
BridgeHandle* bridge_start_audio(const char* device_name);

/// Stop a running bridge and free the handle.
void bridge_stop(BridgeHandle* handle);

/// List available audio devices as newline-separated names.
/// Caller must free with bridge_free_string().
char* bridge_list_audio_devices(void);

/// Free a string returned by the bridge.
void bridge_free_string(char* s);
