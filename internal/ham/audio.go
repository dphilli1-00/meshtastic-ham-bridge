package ham

// AudioDevice implements HamDevice using AFSK Bell 202 over system audio.
// Uses malgo (miniaudio) for cross-platform audio I/O:
// works on Windows, macOS, Linux, Raspberry Pi, Android, iOS.
//
// Bell 202: 1200 baud, mark=1200Hz ('1'), space=2200Hz ('0')
// Framing: HDLC with bit stuffing (AX.25)

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

const (
	baudRate   = 1200.0
	markFreq   = 1200.0
	spaceFreq  = 2200.0
	sampleRate = 44100
)

// AudioDeviceInfo describes an audio device with its index.
type AudioDeviceInfo struct {
	Index int
	Name  string
}

// ListAudioDevices returns names of all available audio devices (deduplicated).
func ListAudioDevices() ([]string, error) {
	devs, err := ListAudioDevicesIndexed()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, d := range devs {
		if !seen[d.Name] {
			seen[d.Name] = true
			names = append(names, d.Name)
		}
	}
	return names, nil
}

// ListAudioOutputsIndexed returns all playback devices with their indices.
func ListAudioOutputsIndexed() ([]AudioDeviceInfo, error) {
	return listAudioIndexed(malgo.Playback)
}

// ListAudioInputsIndexed returns all capture devices with their indices.
func ListAudioInputsIndexed() ([]AudioDeviceInfo, error) {
	return listAudioIndexed(malgo.Capture)
}

// ListAudioDevicesIndexed returns all audio devices with indices (may have duplicate names).
func ListAudioDevicesIndexed() ([]AudioDeviceInfo, error) {
	return listAudioIndexed(malgo.Playback)
}

func listAudioIndexed(kind malgo.DeviceType) ([]AudioDeviceInfo, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}
	defer ctx.Uninit()

	devices, err := ctx.Devices(kind)
	if err != nil {
		return nil, err
	}
	result := make([]AudioDeviceInfo, len(devices))
	for i, d := range devices {
		result[i] = AudioDeviceInfo{Index: i, Name: d.Name()}
	}
	return result, nil
}

type AudioDevice struct {
	mctx      *malgo.AllocatedContext
	inDevice  *malgo.Device
	outDevice *malgo.Device
	recv      chan []byte
	rawBits   chan bool // raw bit decisions before framing, for diagnostics
	demod     *demodulator
	mod       *modulator
	ptt       PTT
	frx       *frameDecoder
}

// Energy returns the current signal energy level (EMA of power).
// Useful for calibrating energyThreshold.
func (a *AudioDevice) Energy() float64 {
	return a.demod.energy
}

// Disc returns the current smoothed FM discriminator output.
// Negative → mark (1200 Hz), Positive → space (2200 Hz).
func (a *AudioDevice) Disc() float64 {
	return a.demod.disc
}

// RawBits returns the channel of raw bit decisions from the demodulator,
// bypassing HDLC framing. Useful for diagnosing the demodulator.
func (a *AudioDevice) RawBits() <-chan bool {
	return a.rawBits
}

// SendTones queues a raw alternating mark/space pattern (n bits each) with no
// HDLC framing. Useful for testing the demodulator in isolation.
func (a *AudioDevice) SendTones(pattern []bool) {
	a.mod.enqueueBits(pattern)
}

// NewAudio opens the named audio device for both TX and RX (substring match).
// Use NewAudioTX / NewAudioRX when TX and RX are on different devices.
func NewAudio(deviceName string) (*AudioDevice, error) {
	return newAudio(deviceName, deviceName)
}

// NewAudioTX opens a TX-only AudioDevice (playback). RecvFrame will block forever.
func NewAudioTX(playbackDevice string) (*AudioDevice, error) {
	return newAudio("", playbackDevice)
}

// NewAudioRX opens an RX-only AudioDevice (capture). SendFrame is a no-op.
func NewAudioRX(captureDevice string) (*AudioDevice, error) {
	return newAudio(captureDevice, "")
}

// newAudio opens capture and/or playback devices by name (substring match).
// Pass "" to skip opening that direction.
func newAudio(captureName, playbackName string) (*AudioDevice, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}

	a := &AudioDevice{
		mctx:    mctx,
		recv:    make(chan []byte, 32),
		rawBits: make(chan bool, 4096),
		demod:   newDemodulator(),
		mod:     newModulator(),
		ptt:     VOXPTT{},        // default: VOX (no-op PTT)
		frx:     newFrameDecoder(), // simple robust framing
	}

	// Capture (mic → demodulator)
	if captureName != "" {
		var inDevID *malgo.DeviceID
		captures, _ := mctx.Devices(malgo.Capture)
		for _, d := range captures {
			if strings.Contains(d.Name(), captureName) {
				id := d.ID
				inDevID = &id
				break
			}
		}
		if inDevID == nil {
			return nil, fmt.Errorf("audio capture device %q not found", captureName)
		}
		captureCfg := malgo.DefaultDeviceConfig(malgo.Capture)
		captureCfg.Capture.Format = malgo.FormatF32
		captureCfg.Capture.Channels = 1
		captureCfg.SampleRate = sampleRate
		captureCfg.Capture.DeviceID = inDevID.Pointer()
		captureCallbacks := malgo.DeviceCallbacks{
			Data: func(_, pInput []byte, frameCount uint32) {
				samples := bytesToFloat32(pInput)
				_, bits := a.demod.pushBits(samples)
				for _, b := range bits {
					select {
					case a.rawBits <- b:
					default:
					}
					if frame := a.frx.push(b); frame != nil {
						select {
						case a.recv <- frame:
						default:
						}
					}
				}
			},
		}
		a.inDevice, err = malgo.InitDevice(mctx.Context, captureCfg, captureCallbacks)
		if err != nil {
			return nil, fmt.Errorf("malgo capture init: %w", err)
		}
		if err := a.inDevice.Start(); err != nil {
			return nil, fmt.Errorf("malgo capture start: %w", err)
		}
	}

	// Playback (modulator → speaker)
	if playbackName != "" {
		var outDevID *malgo.DeviceID
		playbacks, _ := mctx.Devices(malgo.Playback)
		for _, d := range playbacks {
			if strings.Contains(d.Name(), playbackName) {
				id := d.ID
				outDevID = &id
				break
			}
		}
		if outDevID == nil {
			return nil, fmt.Errorf("audio playback device %q not found", playbackName)
		}
		playbackCfg := malgo.DefaultDeviceConfig(malgo.Playback)
		playbackCfg.Playback.Format = malgo.FormatF32
		playbackCfg.Playback.Channels = 1
		playbackCfg.SampleRate = sampleRate
		playbackCfg.Playback.DeviceID = outDevID.Pointer()
		playbackCallbacks := malgo.DeviceCallbacks{
			Data: func(pOutput, _ []byte, frameCount uint32) {
				a.mod.fill(pOutput, int(frameCount))
			},
		}
		a.outDevice, err = malgo.InitDevice(mctx.Context, playbackCfg, playbackCallbacks)
		if err != nil {
			return nil, fmt.Errorf("malgo playback init: %w", err)
		}
		if err := a.outDevice.Start(); err != nil {
			return nil, fmt.Errorf("malgo playback start: %w", err)
		}
	}

	return a, nil
}

// SetPTT replaces the default VOXPTT with an active PTT implementation.
func (a *AudioDevice) SetPTT(p PTT) { a.ptt = p }

func (a *AudioDevice) DeviceType() string { return "audio-afsk" }

// SendFrame keys PTT, queues the AFSK-encoded frame, then releases PTT once
// the modulator drains. The modulator encodes a ~333ms preamble before data,
// so the radio has time to fully key before bits start.
func (a *AudioDevice) SendFrame(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.ptt.On(); err != nil {
		return fmt.Errorf("PTT on: %w", err)
	}
	a.mod.enqueueBits(frameEncode(data))
	// Wait for modulator to drain (preamble + data + tail).
	// At 1200 baud, 50 flags (~333ms) + data. Poll every 10ms.
	for {
		a.mod.mu.Lock()
		pending := len(a.mod.pending)
		a.mod.mu.Unlock()
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			_ = a.ptt.Off()
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return a.ptt.Off()
}

func (a *AudioDevice) RecvFrame(ctx context.Context) ([]byte, error) {
	select {
	case frame := <-a.recv:
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *AudioDevice) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (a *AudioDevice) Close() error {
	if a.inDevice != nil {
		a.inDevice.Stop()
		a.inDevice.Uninit()
	}
	if a.outDevice != nil {
		a.outDevice.Stop()
		a.outDevice.Uninit()
	}
	a.mctx.Uninit()
	return nil
}

// ---------------------------------------------------------------------------
// Bell 202 AFSK modulator
// ---------------------------------------------------------------------------

type modulator struct {
	mu      sync.Mutex
	phase   float64
	pending []float32
}

func newModulator() *modulator { return &modulator{} }

// enqueueBits queues raw bits (true=mark/1200Hz, false=space/2200Hz) with no framing.
func (m *modulator) enqueueBits(bits []bool) {
	samples := m.bitsToSamples(bits)
	m.mu.Lock()
	m.pending = append(m.pending, samples...)
	m.mu.Unlock()
}

func (m *modulator) enqueue(data []byte) {
	samples := m.bitsToSamples(hdlcEncode(data))
	m.mu.Lock()
	m.pending = append(m.pending, samples...)
	m.mu.Unlock()
}

func (m *modulator) bitsToSamples(bits []bool) []float32 {
	spb := float64(sampleRate) / baudRate
	out := make([]float32, 0, int(float64(len(bits))*spb))
	for _, bit := range bits {
		freq := spaceFreq
		if bit {
			freq = markFreq
		}
		n := int(spb)
		for i := 0; i < n; i++ {
			out = append(out, float32(math.Sin(2*math.Pi*m.phase)*0.7))
			m.phase += freq / float64(sampleRate)
			if m.phase >= 1.0 {
				m.phase -= 1.0
			}
		}
	}
	return out
}

func (m *modulator) fill(out []byte, frames int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := 0; i < frames; i++ {
		var s float32
		if len(m.pending) > 0 {
			s = m.pending[0]
			m.pending = m.pending[1:]
		}
		b := math.Float32bits(s)
		out[i*4]   = byte(b)
		out[i*4+1] = byte(b >> 8)
		out[i*4+2] = byte(b >> 16)
		out[i*4+3] = byte(b >> 24)
	}
}

// ---------------------------------------------------------------------------
// Bell 202 AFSK demodulator — FM discriminator approach
// ---------------------------------------------------------------------------
//
// Architecture:
//   1. Quadrature mix input to baseband at center frequency (1700 Hz).
//   2. LPF I and Q channels (cutoff ~700 Hz, ≈ half the 1000 Hz deviation).
//   3. FM discriminator: freq = Im(conj(z[n-1]) * z[n]) = Q_prev*I - I_prev*Q.
//      disc < 0 → below center (1200 Hz mark → bit 1)
//      disc > 0 → above center (2200 Hz space → bit 0)
//   4. LPF discriminator output to reject noise.
//   5. Clock recovery: on disc sign change (bit transition), nudge bitClock
//      toward a bit boundary (bitClock ≈ 0).
//   6. Energy squelch via EMA of input power.
//
// Advantage over bandpass filter approach: discriminator output tracks
// instantaneous frequency and doesn't have the saturation/memory problem
// of narrow resonators. Works correctly regardless of tone amplitude ratio
// (immune to FM pre-emphasis amplitude effects).

const (
	centerFreq      = (markFreq + spaceFreq) / 2 // 1700 Hz
	energyThreshold = 0.005                       // EMA power; ≈ RMS 0.07
	lpfAlpha        = 0.1                         // IQ lowpass cutoff ~700 Hz at 44100
	discAlpha       = 0.15                        // discriminator smoothing
)

type demodulator struct {
	spb      float64
	bitClock float64

	// Quadrature mixer phase
	centerPhase float64

	// IQ lowpass state
	iLP, qLP float64
	// previous filtered IQ for discriminator
	iPrev, qPrev float64
	// smoothed discriminator output
	disc float64
	// previous disc sign for transition detection
	prevSign int

	// Clock recovery
	energy float64 // EMA of input power

	// HDLC state
	shiftReg  uint8
	onesCount int
	inFrame   bool
	frameBuf  []byte
	curByte   uint8
	curBit    uint8
}

func newDemodulator() *demodulator {
	return &demodulator{spb: float64(sampleRate) / baudRate}
}

func sign(x float64) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

// pushBits processes samples and returns completed HDLC frames plus every
// raw bit decision (before HDLC state machine). Raw bits are useful for
// diagnosing demodulator issues without framing getting in the way.
func (d *demodulator) pushBits(samples []float32) (frames [][]byte, bits []bool) {
	for _, s := range samples {
		fs := float64(s)

		// Energy squelch (EMA of power).
		d.energy = d.energy*0.999 + fs*fs*0.001
		if d.energy < energyThreshold {
			d.resetFrame()
			d.iLP, d.qLP = 0, 0
			d.iPrev, d.qPrev = 0, 0
			d.disc = 0
			d.bitClock = 0
			d.prevSign = 0
			d.centerPhase = 0
			continue
		}

		// Step 1: quadrature mix to baseband at centerFreq.
		angle := 2 * math.Pi * d.centerPhase * centerFreq / float64(sampleRate)
		iRaw := fs * math.Cos(angle)
		qRaw := fs * math.Sin(angle)
		d.centerPhase++

		// Step 2: IQ lowpass filter.
		d.iLP += lpfAlpha * (iRaw - d.iLP)
		d.qLP += lpfAlpha * (qRaw - d.qLP)

		// Step 3: FM discriminator — instantaneous frequency.
		// Im(conj(z_prev) * z_cur) = Q_prev*I_cur - I_prev*Q_cur
		raw := d.qPrev*d.iLP - d.iPrev*d.qLP
		d.iPrev, d.qPrev = d.iLP, d.qLP

		// Step 4: smooth discriminator output.
		d.disc += discAlpha * (raw - d.disc)

		// Step 5: bit decision.
		// disc < 0 → mark (1200 Hz, below center) → bit true
		// disc > 0 → space (2200 Hz, above center) → bit false
		bit := d.disc < 0
		sg := sign(d.disc)

		// Clock recovery: disc sign change = bit transition = bit boundary.
		if sg != 0 && sg != d.prevSign && d.prevSign != 0 {
			// Ideal: transition at bitClock ≈ 0 (just had boundary).
			// Error: how far we are from the nearest boundary.
			err := d.bitClock
			if err > d.spb/2 {
				err -= d.spb // wrap to [-spb/2, spb/2]
			}
			d.bitClock -= err * 0.35
			if d.bitClock < 0 {
				d.bitClock += d.spb
			}
		}
		d.prevSign = sg

		// Sample bit at clock boundary.
		d.bitClock++
		if d.bitClock >= d.spb {
			d.bitClock -= d.spb
			bits = append(bits, bit)
		}
	}
	return frames, bits
}

func (d *demodulator) resetFrame() {
	d.inFrame = false
	if d.frameBuf != nil {
		d.frameBuf = d.frameBuf[:0]
	}
	d.shiftReg = 0
	d.curByte = 0
	d.curBit = 0
	d.onesCount = 0
}

func (d *demodulator) pushBit(bit bool) []byte {
	var b uint8
	if bit {
		b = 1
	}
	d.shiftReg = d.shiftReg>>1 | b<<7

	if d.shiftReg == 0x7E {
		if d.inFrame && len(d.frameBuf) > 2 {
			frame := append([]byte(nil), d.frameBuf...)
			d.resetFrame()
			return frame
		}
		// Flag byte = start of frame (or inter-frame fill).
		d.inFrame = true
		d.frameBuf = d.frameBuf[:0]
		d.onesCount = 0
		d.curByte = 0
		d.curBit = 0
		return nil
	}
	if !d.inFrame {
		return nil
	}
	if bit {
		d.onesCount++
	} else {
		if d.onesCount >= 5 {
			// Destuffed zero — drop it and continue.
			d.onesCount = 0
			return nil
		}
		d.onesCount = 0
	}
	d.curByte |= b << d.curBit
	d.curBit++
	if d.curBit == 8 {
		d.frameBuf = append(d.frameBuf, d.curByte)
		d.curByte = 0
		d.curBit = 0
	}
	return nil
}

// ---------------------------------------------------------------------------
// HDLC helpers
// ---------------------------------------------------------------------------

func hdlcEncode(data []byte) []bool {
	var bits []bool
	for i := 0; i < 50; i++ { // 50 flags ≈ 333ms preamble, gives VOX time to key
		bits = append(bits, flagBits()...)
	}
	var ones int
	for _, b := range data {
		for i := 0; i < 8; i++ {
			bit := (b>>i)&1 == 1
			bits = append(bits, bit)
			if bit {
				ones++
				if ones == 5 {
					bits = append(bits, false)
					ones = 0
				}
			} else {
				ones = 0
			}
		}
	}
	bits = append(bits, flagBits()...)
	return bits
}

func flagBits() []bool {
	bits := make([]bool, 8)
	for i := 0; i < 8; i++ {
		bits[i] = (0x7E>>i)&1 == 1
	}
	return bits
}

func bytesToFloat32(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		out[i] = math.Float32frombits(bits)
	}
	return out
}
