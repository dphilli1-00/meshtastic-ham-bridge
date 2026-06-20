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

	"github.com/gen2brain/malgo"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

const (
	baudRate   = 1200.0
	markFreq   = 1200.0
	spaceFreq  = 2200.0
	sampleRate = 44100
)

// ListAudioDevices returns names of all available audio devices.
func ListAudioDevices() ([]string, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}
	defer ctx.Uninit()

	devices, err := ctx.Devices(malgo.Playback)
	if err != nil {
		return nil, err
	}
	captureDevices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, d := range append(devices, captureDevices...) {
		name := d.Name()
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

type AudioDevice struct {
	mctx      *malgo.AllocatedContext
	inDevice  *malgo.Device
	outDevice *malgo.Device
	recv      chan []byte
	demod     *demodulator
	mod       *modulator
}

// NewAudio opens the named audio device (substring match) or system default if name is "".
func NewAudio(deviceName string) (*AudioDevice, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}

	a := &AudioDevice{
		mctx:  mctx,
		recv:  make(chan []byte, 32),
		demod: newDemodulator(),
		mod:   newModulator(),
	}

	// Find device if name specified
	var inDevID, outDevID *malgo.DeviceID
	if deviceName != "" {
		captures, _ := mctx.Devices(malgo.Capture)
		for _, d := range captures {
			if strings.Contains(d.Name(), deviceName) {
				id := d.ID
				inDevID = &id
				break
			}
		}
		playbacks, _ := mctx.Devices(malgo.Playback)
		for _, d := range playbacks {
			if strings.Contains(d.Name(), deviceName) {
				id := d.ID
				outDevID = &id
				break
			}
		}
		if inDevID == nil || outDevID == nil {
			return nil, fmt.Errorf("audio device %q not found", deviceName)
		}
	}

	// Capture (mic → demodulator)
	captureCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	captureCfg.Capture.Format = malgo.FormatF32
	captureCfg.Capture.Channels = 1
	captureCfg.SampleRate = sampleRate
	if inDevID != nil {
		captureCfg.Capture.DeviceID = inDevID.Pointer()
	}
	captureCallbacks := malgo.DeviceCallbacks{
		Data: func(_, pInput []byte, frameCount uint32) {
			samples := bytesToFloat32(pInput)
			for _, frame := range a.demod.push(samples) {
				select {
				case a.recv <- frame:
				default:
				}
			}
		},
	}
	a.inDevice, err = malgo.InitDevice(mctx.Context, captureCfg, captureCallbacks)
	if err != nil {
		return nil, fmt.Errorf("malgo capture init: %w", err)
	}

	// Playback (modulator → speaker)
	playbackCfg := malgo.DefaultDeviceConfig(malgo.Playback)
	playbackCfg.Playback.Format = malgo.FormatF32
	playbackCfg.Playback.Channels = 1
	playbackCfg.SampleRate = sampleRate
	if outDevID != nil {
		playbackCfg.Playback.DeviceID = outDevID.Pointer()
	}
	playbackCallbacks := malgo.DeviceCallbacks{
		Data: func(pOutput, _ []byte, frameCount uint32) {
			a.mod.fill(pOutput, int(frameCount))
		},
	}
	a.outDevice, err = malgo.InitDevice(mctx.Context, playbackCfg, playbackCallbacks)
	if err != nil {
		return nil, fmt.Errorf("malgo playback init: %w", err)
	}

	if err := a.inDevice.Start(); err != nil {
		return nil, fmt.Errorf("malgo capture start: %w", err)
	}
	if err := a.outDevice.Start(); err != nil {
		return nil, fmt.Errorf("malgo playback start: %w", err)
	}
	return a, nil
}

func (a *AudioDevice) DeviceType() string { return "audio-afsk" }

func (a *AudioDevice) SendFrame(ctx context.Context, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		a.mod.enqueue(data)
		return nil
	}
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
	a.inDevice.Stop()
	a.outDevice.Stop()
	a.inDevice.Uninit()
	a.outDevice.Uninit()
	a.mctx.Uninit()
	return nil
}

// ---------------------------------------------------------------------------
// Bell 202 AFSK modulator
// ---------------------------------------------------------------------------

type modulator struct {
	phase   float64
	pending []float32
}

func newModulator() *modulator { return &modulator{} }

func (m *modulator) enqueue(data []byte) {
	bits := hdlcEncode(data)
	spb := float64(sampleRate) / baudRate
	for _, bit := range bits {
		freq := spaceFreq
		if bit {
			freq = markFreq
		}
		n := int(spb)
		for i := 0; i < n; i++ {
			m.pending = append(m.pending, float32(math.Sin(2*math.Pi*m.phase)*0.7))
			m.phase += freq / float64(sampleRate)
			if m.phase >= 1.0 {
				m.phase -= 1.0
			}
		}
	}
}

func (m *modulator) fill(out []byte, frames int) {
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
// Bell 202 AFSK demodulator
// ---------------------------------------------------------------------------

type demodulator struct {
	spb            float64
	bitClock       float64
	markI, markQ   float64
	spaceI, spaceQ float64
	phase          float64
	shiftReg       uint8
	onesCount      int
	inFrame        bool
	frameBuf       []byte
	curByte        uint8
	curBit         uint8
}

func newDemodulator() *demodulator {
	return &demodulator{spb: float64(sampleRate) / baudRate}
}

func (d *demodulator) push(samples []float32) [][]byte {
	var frames [][]byte
	for _, s := range samples {
		mp := d.phase * markFreq / float64(sampleRate) * 2 * math.Pi
		sp := d.phase * spaceFreq / float64(sampleRate) * 2 * math.Pi
		fs := float64(s)
		d.markI  = d.markI*0.99  + fs*math.Cos(mp)*0.01
		d.markQ  = d.markQ*0.99  + fs*math.Sin(mp)*0.01
		d.spaceI = d.spaceI*0.99 + fs*math.Cos(sp)*0.01
		d.spaceQ = d.spaceQ*0.99 + fs*math.Sin(sp)*0.01
		d.phase++

		bit := (d.markI*d.markI + d.markQ*d.markQ) > (d.spaceI*d.spaceI + d.spaceQ*d.spaceQ)
		d.bitClock++
		if d.bitClock >= d.spb {
			d.bitClock -= d.spb
			if frame := d.pushBit(bit); frame != nil {
				frames = append(frames, frame)
			}
		}
	}
	return frames
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
			d.frameBuf = d.frameBuf[:0]
			d.inFrame = false
			d.onesCount = 0
			d.curByte = 0
			d.curBit = 0
			return frame
		}
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
	for i := 0; i < 20; i++ {
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
