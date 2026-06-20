/// AudioHamDevice — AFSK Bell 202 soft modem over system audio.
///
/// Works on any platform with audio I/O: desktop (via Digirig / audio cable),
/// iOS (via Lightning/USB-C audio adapter), Android (same).
/// On iOS this is what replaces Direwolf — no separate process needed.
///
/// Protocol:
///   - Bell 202 AFSK: 1200 baud, mark=1200 Hz, space=2200 Hz
///   - AX.25 frames, HDLC framing (flag=0x7E, bit stuffing)
///   - KISS framing layer on top (same as direwolf.rs)
///
/// PTT: on desktop, handled by Digirig's VOX or RTS pin.
///      On iOS/Android, VOX only (no serial port for RTS).

use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::StreamConfig;
use tokio::sync::{mpsc, Mutex};
use async_trait::async_trait;
use crate::error::{Error, Result as HamResult};
use crate::types::DeviceStatus;
use super::HamDevice;

// Bell 202 constants
const BAUD_RATE: f32 = 1200.0;
const MARK_FREQ: f32 = 1200.0;  // '1' bit
const SPACE_FREQ: f32 = 2200.0; // '0' bit
const SAMPLE_RATE: u32 = 44100;

// cpal::Stream holds raw C pointers and doesn't implement Send/Sync, but it's
// safe to hold across threads as long as we don't call play/pause from multiple
// threads simultaneously. We just need to keep it alive.
struct SendStream(cpal::Stream);
unsafe impl Send for SendStream {}
unsafe impl Sync for SendStream {}

// Safety: all shared state goes through Mutex/mpsc; streams are only touched
// by the audio callbacks which run on the audio thread.
unsafe impl Send for AudioHamDevice {}
unsafe impl Sync for AudioHamDevice {}

pub struct AudioHamDevice {
    // Frames decoded from audio land here; recv_frame() reads from this
    rx: Mutex<mpsc::Receiver<Vec<u8>>>,
    // Frames to transmit go here; audio output thread reads from this
    tx: mpsc::Sender<Vec<u8>>,
    // Keep streams alive — dropped = stream stops
    _input_stream: SendStream,
    _output_stream: SendStream,
}

impl AudioHamDevice {
    /// Open the default audio input/output device and start the modem.
    /// `device_name`: None = system default, Some("Built-in Microphone") etc.
    pub fn new(device_name: Option<&str>) -> HamResult<Self> {
        let host = cpal::default_host();

        let input_device = match device_name {
            None => host.default_input_device()
                .ok_or_else(|| Error::Unknown("no audio input device".into()))?,
            Some(name) => host.input_devices()
                .map_err(|e| Error::Unknown(e.to_string()))?
                .find(|d| d.name().map(|n| n.contains(name)).unwrap_or(false))
                .ok_or_else(|| Error::Unknown(format!("audio device '{}' not found", name)))?,
        };

        let output_device = match device_name {
            None => host.default_output_device()
                .ok_or_else(|| Error::Unknown("no audio output device".into()))?,
            Some(name) => host.output_devices()
                .map_err(|e| Error::Unknown(e.to_string()))?
                .find(|d| d.name().map(|n| n.contains(name)).unwrap_or(false))
                .ok_or_else(|| Error::Unknown(format!("audio device '{}' not found", name)))?,
        };

        let config = StreamConfig {
            channels: 1,
            sample_rate: cpal::SampleRate(SAMPLE_RATE),
            buffer_size: cpal::BufferSize::Default,
        };

        // Channel: decoded frames from demodulator → recv_frame()
        let (frame_tx, frame_rx) = mpsc::channel::<Vec<u8>>(32);
        // Channel: frames to transmit → modulator → audio out
        let (tx_tx, mut tx_rx) = mpsc::channel::<Vec<u8>>(32);

        // --- Input: demodulate audio → AX.25 frames ---
        let mut demod = Demodulator::new(SAMPLE_RATE);
        let input_stream = input_device.build_input_stream(
            &config,
            move |data: &[f32], _| {
                for frame in demod.push_samples(data) {
                    let _ = frame_tx.try_send(frame);
                }
            },
            |e| eprintln!("audio input error: {e}"),
            None,
        ).map_err(|e| Error::Unknown(e.to_string()))?;

        // --- Output: frames → modulate → audio ---
        let mut modulator = Modulator::new(SAMPLE_RATE);
        let mut pending_samples: Vec<f32> = Vec::new();
        let output_stream = output_device.build_output_stream(
            &config,
            move |data: &mut [f32], _| {
                // Pull any newly queued frames
                while let Ok(frame) = tx_rx.try_recv() {
                    pending_samples.extend(modulator.modulate(&frame));
                }
                // Fill output buffer
                for sample in data.iter_mut() {
                    *sample = if pending_samples.is_empty() {
                        0.0
                    } else {
                        pending_samples.remove(0)
                    };
                }
            },
            |e| eprintln!("audio output error: {e}"),
            None,
        ).map_err(|e| Error::Unknown(e.to_string()))?;

        input_stream.play().map_err(|e| Error::Unknown(e.to_string()))?;
        output_stream.play().map_err(|e| Error::Unknown(e.to_string()))?;

        Ok(Self {
            rx: Mutex::new(frame_rx),
            tx: tx_tx,
            _input_stream: SendStream(input_stream),
            _output_stream: SendStream(output_stream),
        })
    }

    /// List available audio devices — useful for device discovery / CLI.
    pub fn list_devices() -> HamResult<Vec<String>> {
        let host = cpal::default_host();
        let devices = host.devices()
            .map_err(|e| Error::Unknown(e.to_string()))?
            .filter_map(|d| d.name().ok())
            .collect();
        Ok(devices)
    }
}

#[async_trait]
impl HamDevice for AudioHamDevice {
    fn device_type(&self) -> &str { "audio-afsk" }

    async fn send_frame(&self, data: &[u8]) -> HamResult<()> {
        self.tx.send(data.to_vec()).await
            .map_err(|e| Error::Unknown(e.to_string()))
    }

    async fn recv_frame(&self) -> HamResult<Vec<u8>> {
        let mut rx = self.rx.lock().await;
        rx.recv().await.ok_or_else(|| Error::Unknown("audio rx closed".into()))
    }

    async fn status(&self) -> HamResult<DeviceStatus> {
        Ok(DeviceStatus::connected())
    }

    async fn disconnect(&self) -> HamResult<()> {
        // Dropping _input_stream/_output_stream stops the audio.
        // Since they're owned by self and self is dropped when bridge drops us, this is automatic.
        Ok(())
    }
}

// ---------------------------------------------------------------------------
// Bell 202 AFSK Modulator
// Encodes bytes → HDLC bits → AFSK audio samples
// ---------------------------------------------------------------------------

struct Modulator {
    sample_rate: u32,
    phase: f32,
}

impl Modulator {
    fn new(sample_rate: u32) -> Self {
        Self { sample_rate, phase: 0.0 }
    }

    /// Encode `data` as a KISS frame, then HDLC, then AFSK audio samples.
    fn modulate(&mut self, data: &[u8]) -> Vec<f32> {
        let bits = hdlc_encode(data);
        let samples_per_bit = self.sample_rate as f32 / BAUD_RATE;
        let mut samples = Vec::new();

        // Preamble: 300ms of flag bytes (0x7E) before frame
        let preamble_bits = hdlc_encode(&vec![0x7Eu8; ((BAUD_RATE * 0.3) / 8.0) as usize]);
        for bit in preamble_bits.iter().chain(bits.iter()) {
            let freq = if *bit { MARK_FREQ } else { SPACE_FREQ };
            let n = samples_per_bit as usize;
            for _ in 0..n {
                samples.push((self.phase * 2.0 * std::f32::consts::PI).sin() * 0.7);
                self.phase += freq / self.sample_rate as f32;
                if self.phase >= 1.0 { self.phase -= 1.0; }
            }
        }

        samples
    }
}

// ---------------------------------------------------------------------------
// Bell 202 AFSK Demodulator
// Audio samples → HDLC bits → AX.25 frames
// ---------------------------------------------------------------------------

struct Demodulator {
    sample_rate: u32,
    // Correlator state
    mark_i: f32, mark_q: f32,
    space_i: f32, space_q: f32,
    phase: f32,
    bit_clock: f32,
    samples_per_bit: f32,
    // HDLC state
    shift_reg: u8,
    bit_count: u8,
    ones_count: u8,       // for bit destuffing
    in_frame: bool,
    frame_buf: Vec<u8>,
    current_byte: u8,
    current_bit: u8,
}

impl Demodulator {
    fn new(sample_rate: u32) -> Self {
        Self {
            sample_rate,
            mark_i: 0.0, mark_q: 0.0,
            space_i: 0.0, space_q: 0.0,
            phase: 0.0,
            bit_clock: 0.0,
            samples_per_bit: sample_rate as f32 / BAUD_RATE,
            shift_reg: 0,
            bit_count: 0,
            ones_count: 0,
            in_frame: false,
            frame_buf: Vec::new(),
            current_byte: 0,
            current_bit: 0,
        }
    }

    /// Feed audio samples, returns any complete frames decoded.
    fn push_samples(&mut self, samples: &[f32]) -> Vec<Vec<u8>> {
        let mut frames = Vec::new();
        for &s in samples {
            // IQ correlators for mark and space
            let mark_phase = self.phase * MARK_FREQ / self.sample_rate as f32 * 2.0 * std::f32::consts::PI;
            let space_phase = self.phase * SPACE_FREQ / self.sample_rate as f32 * 2.0 * std::f32::consts::PI;
            self.mark_i  = self.mark_i  * 0.99 + s * mark_phase.cos()  * 0.01;
            self.mark_q  = self.mark_q  * 0.99 + s * mark_phase.sin()  * 0.01;
            self.space_i = self.space_i * 0.99 + s * space_phase.cos() * 0.01;
            self.space_q = self.space_q * 0.99 + s * space_phase.sin() * 0.01;
            self.phase += 1.0;

            let mark_power  = self.mark_i  * self.mark_i  + self.mark_q  * self.mark_q;
            let space_power = self.space_i * self.space_i + self.space_q * self.space_q;
            let bit = mark_power > space_power; // true = mark = '1'

            // Simple bit clock: sample at center of each bit period
            self.bit_clock += 1.0;
            if self.bit_clock >= self.samples_per_bit {
                self.bit_clock -= self.samples_per_bit;
                if let Some(frame) = self.push_bit(bit) {
                    frames.push(frame);
                }
            }
        }
        frames
    }

    fn push_bit(&mut self, bit: bool) -> Option<Vec<u8>> {
        // NRZI decode: bit flip = 0, no flip = 1
        // (tracked implicitly: AFSK mark=1, space=0 is already NRZ here for simplicity)
        // TODO: implement proper NRZI

        self.shift_reg = (self.shift_reg >> 1) | if bit { 0x80 } else { 0x00 };
        self.bit_count += 1;

        if self.shift_reg == 0x7E && self.bit_count >= 8 {
            // HDLC flag
            if self.in_frame && self.frame_buf.len() > 2 {
                let frame = self.frame_buf.clone();
                self.frame_buf.clear();
                self.in_frame = false;
                self.ones_count = 0;
                return Some(frame);
            }
            self.in_frame = true;
            self.frame_buf.clear();
            self.ones_count = 0;
            self.current_byte = 0;
            self.current_bit = 0;
            return None;
        }

        if !self.in_frame { return None; }

        // Bit destuffing: after 5 consecutive 1s, next 0 is stuffed, discard it
        if bit {
            self.ones_count += 1;
        } else {
            if self.ones_count >= 5 {
                self.ones_count = 0;
                return None; // stuffed zero, discard
            }
            self.ones_count = 0;
        }

        // Accumulate bits into bytes (LSB first for AX.25)
        self.current_byte |= if bit { 1 << self.current_bit } else { 0 };
        self.current_bit += 1;
        if self.current_bit == 8 {
            self.frame_buf.push(self.current_byte);
            self.current_byte = 0;
            self.current_bit = 0;
        }

        None
    }
}

// ---------------------------------------------------------------------------
// HDLC encoding helpers
// ---------------------------------------------------------------------------

/// Wrap `data` in HDLC: flag, bit-stuffed payload, flag.
fn hdlc_encode(data: &[u8]) -> Vec<bool> {
    let mut bits: Vec<bool> = Vec::new();

    // Opening flag
    for i in 0..8 {
        bits.push((0x7Eu8 >> i) & 1 == 1);
    }

    // Data with bit stuffing (insert 0 after 5 consecutive 1s)
    let mut ones = 0u8;
    for byte in data {
        for i in 0..8 {
            let bit = (byte >> i) & 1 == 1;
            bits.push(bit);
            if bit {
                ones += 1;
                if ones == 5 {
                    bits.push(false); // stuff a zero
                    ones = 0;
                }
            } else {
                ones = 0;
            }
        }
    }

    // Closing flag
    for i in 0..8 {
        bits.push((0x7Eu8 >> i) & 1 == 1);
    }

    bits
}
