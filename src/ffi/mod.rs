/// C-compatible FFI layer for iOS/Android.
///
/// Build the Rust core as a static library:
///   cargo build --target aarch64-apple-ios --release
///   cargo build --target aarch64-linux-android --release
///
/// Then link it into the Swift/Kotlin app and call these functions.
/// Generate the C header with: cargo install cbindgen && cbindgen --output bridge.h
///
/// iOS usage (Swift):
///   1. Add libmeshtastic_ham_bridge.a to Xcode target
///   2. Add bridge.h to bridging header
///   3. Call bridge_start() on app launch, bridge_stop() on background

use std::ffi::{CStr, CString};
use std::os::raw::c_char;

/// Opaque handle to a running bridge instance.
pub struct BridgeHandle {
    // Will hold the tokio runtime + bridge Arc when implemented
    _private: (),
}

/// Start the bridge with audio modem on both mesh and ham sides.
/// Returns null on failure.
/// device_name: null = system default audio device
#[no_mangle]
pub extern "C" fn bridge_start_audio(device_name: *const c_char) -> *mut BridgeHandle {
    let _name = if device_name.is_null() {
        None
    } else {
        unsafe { CStr::from_ptr(device_name).to_str().ok() }
    };

    // TODO: spin up tokio runtime, create AudioHamDevice + MeshDevice, run bridge
    // For now return null (not yet implemented)
    std::ptr::null_mut()
}

/// Stop a running bridge and free the handle.
#[no_mangle]
pub extern "C" fn bridge_stop(handle: *mut BridgeHandle) {
    if !handle.is_null() {
        unsafe { drop(Box::from_raw(handle)); }
    }
}

/// List available audio devices as newline-separated names.
/// Caller must free the returned string with bridge_free_string().
#[no_mangle]
pub extern "C" fn bridge_list_audio_devices() -> *mut c_char {
    match crate::ham::audio::AudioHamDevice::list_devices() {
        Ok(devices) => {
            let joined = devices.join("\n");
            CString::new(joined).map(|s| s.into_raw()).unwrap_or(std::ptr::null_mut())
        }
        Err(_) => std::ptr::null_mut(),
    }
}

/// Free a string returned by the bridge.
#[no_mangle]
pub extern "C" fn bridge_free_string(s: *mut c_char) {
    if !s.is_null() {
        unsafe { drop(CString::from_raw(s)); }
    }
}
