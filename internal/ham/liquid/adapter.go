//go:build liquid

package liquid

// Adapter implements ham.Device using the liquid-dsp OFDM flex frame modem
// for audio I/O and the platform PTT interface.
//
// Audio chain (TX):
//   []byte payload → Modem.TXSamples() → IQ → downmix to mono → malgo playback
//
// Audio chain (RX):
//   malgo capture → mono → upcast to IQ (imag=0) → Modem.PushRXSamples() → rxCh
//
// The downmix from IQ to mono audio:
//   audio[n] = I[n]*cos(2π*fc*n/fs) - Q[n]*sin(2π*fc*n/fs)
// where fc = centre frequency (default 1500 Hz — mid audio band).
// For VHF FM radios this is just a tone pair; for HF SSB it becomes the
// actual transmitted SSB signal (the math is equivalent to USB modulation).

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/types"
)

const (
	sampleRate    = 48000          // Hz — matches most USB audio devices
	centreFreqHz  = 1500.0         // IQ downmix carrier (mid audio band)
	txQueueDepth  = 8
	frameTimeout  = 5 * time.Second
)

// Adapter wraps Modem + malgo audio I/O + PTT into a ham.Device.
type Adapter struct {
	modem   *Modem
	ptt     ham.PTT
	ctx     *malgo.AllocatedContext
	playDev malgo.Device
	capDev  malgo.Device

	txMu    sync.Mutex
	txQueue chan []float32 // chunks of mono float32 ready to play

	sampleIdx uint64 // monotonic sample counter for IQ downmix phase
}

// NewAdapter creates an Adapter using the named audio output and input devices.
// outName / inName: substring match against malgo device names. Pass "" for system default.
// ptt: PTT implementation (VOXPTT, CM108PTT, etc.)
func NewAdapter(outName, inName string, ptt ham.PTT) (*Adapter, error) {
	modem, err := NewModem(DefaultSubcarriers, DefaultCPLen, DefaultTaperLen)
	if err != nil {
		return nil, err
	}

	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		modem.Close()
		return nil, fmt.Errorf("malgo init: %w", err)
	}

	a := &Adapter{
		modem:   modem,
		ptt:     ptt,
		ctx:     mctx,
		txQueue: make(chan []float32, txQueueDepth),
	}

	if err := a.initPlayback(outName); err != nil {
		modem.Close()
		_ = mctx.Uninit()
		return nil, err
	}
	if err := a.initCapture(inName); err != nil {
		a.playDev.Uninit()
		modem.Close()
		_ = mctx.Uninit()
		return nil, err
	}

	_ = a.playDev.Start()
	_ = a.capDev.Start()
	return a, nil
}

func (a *Adapter) initPlayback(name string) error {
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = 1
	cfg.SampleRate = sampleRate

	if id := findDevice(a.ctx, malgo.Playback, name); id != nil {
		cfg.Playback.DeviceID = id.Pointer()
	}

	cb := malgo.DeviceCallbacks{
		Data: func(out, _ []byte, frameCount uint32) {
			a.fillPlayback(out, frameCount)
		},
	}
	dev, err := malgo.InitDevice(a.ctx.Context, cfg, cb)
	if err != nil {
		return fmt.Errorf("malgo playback init: %w", err)
	}
	a.playDev = *dev
	return nil
}

func (a *Adapter) initCapture(name string) error {
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = sampleRate

	if id := findDevice(a.ctx, malgo.Capture, name); id != nil {
		cfg.Capture.DeviceID = id.Pointer()
	}

	cb := malgo.DeviceCallbacks{
		Data: func(_, in []byte, frameCount uint32) {
			a.processCapture(in, frameCount)
		},
	}
	dev, err := malgo.InitDevice(a.ctx.Context, cfg, cb)
	if err != nil {
		return fmt.Errorf("malgo capture init: %w", err)
	}
	a.capDev = *dev
	return nil
}

// fillPlayback is the malgo playback callback — runs on audio thread.
// Pulls mono float32 frames from txQueue and writes them to the output buffer.
func (a *Adapter) fillPlayback(out []byte, frameCount uint32) {
	needed := int(frameCount)
	pos := 0
	for needed > 0 {
		select {
		case chunk := <-a.txQueue:
			n := needed
			if n > len(chunk) {
				n = len(chunk)
			}
			for i := 0; i < n; i++ {
				v := chunk[i]
				// float32 to 4 bytes little-endian
				bits := math.Float32bits(v)
				out[pos*4+0] = byte(bits)
				out[pos*4+1] = byte(bits >> 8)
				out[pos*4+2] = byte(bits >> 16)
				out[pos*4+3] = byte(bits >> 24)
				pos++
			}
			needed -= n
			if n < len(chunk) {
				// push remainder back — non-blocking best effort
				select {
				case a.txQueue <- chunk[n:]:
				default:
				}
			}
		default:
			// silence
			for i := pos; i < pos+needed; i++ {
				out[i*4] = 0; out[i*4+1] = 0; out[i*4+2] = 0; out[i*4+3] = 0
			}
			return
		}
	}
}

// processCapture is the malgo capture callback — runs on audio thread.
// Upconverts mono audio to IQ (imaginary = 0) and feeds the synchroniser.
func (a *Adapter) processCapture(in []byte, frameCount uint32) {
	n := int(frameCount)
	iq := make([]float32, n*2)
	fc := centreFreqHz
	fs := float64(sampleRate)
	base := float64(a.sampleIdx)
	for i := 0; i < n; i++ {
		// unpack float32 LE
		bits := uint32(in[i*4]) | uint32(in[i*4+1])<<8 | uint32(in[i*4+2])<<16 | uint32(in[i*4+3])<<24
		s := math.Float32frombits(bits)
		phase := 2 * math.Pi * fc * (base + float64(i)) / fs
		iq[i*2+0] = s * float32(math.Cos(phase)) // I
		iq[i*2+1] = s * float32(-math.Sin(phase)) // Q
	}
	a.sampleIdx += uint64(n)
	a.modem.PushRXSamples(iq)
}

// iqToMono downmixes IQ float32 pairs to a mono float32 slice.
// Applies the carrier at centreFreqHz starting from sample offset.
func (a *Adapter) iqToMono(iq []float32, offset uint64) []float32 {
	n := len(iq) / 2
	out := make([]float32, n)
	fc := centreFreqHz
	fs := float64(sampleRate)
	for i := 0; i < n; i++ {
		I := float64(iq[i*2])
		Q := float64(iq[i*2+1])
		phase := 2 * math.Pi * fc * (float64(offset) + float64(i)) / fs
		out[i] = float32(I*math.Cos(phase) - Q*math.Sin(phase))
	}
	return out
}

// ── ham.Device interface ────────────────────────────────────────────────────

func (a *Adapter) DeviceType() string { return "liquid-ofdm" }

// SendFrame encodes payload and transmits it over the audio + PTT path.
func (a *Adapter) SendFrame(_ context.Context, data []byte) error {
	iq, err := a.modem.TXSamples(data)
	if err != nil {
		return fmt.Errorf("liquid TX: %w", err)
	}

	a.txMu.Lock()
	offset := a.sampleIdx // snapshot before PTT
	a.txMu.Unlock()

	mono := a.iqToMono(iq, offset)

	if err := a.ptt.On(); err != nil {
		return fmt.Errorf("PTT on: %w", err)
	}

	// Chunk mono into txQueue in 1024-sample pieces so fillPlayback drains
	// smoothly without holding a large allocation.
	const chunkSize = 1024
	for i := 0; i < len(mono); i += chunkSize {
		end := i + chunkSize
		if end > len(mono) {
			end = len(mono)
		}
		a.txQueue <- mono[i:end]
	}

	// Wait until queue drains before dropping PTT.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(frameTimeout)
	for {
		<-ticker.C
		if len(a.txQueue) == 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}

	if err := a.ptt.Off(); err != nil {
		return fmt.Errorf("PTT off: %w", err)
	}
	return nil
}

// RecvFrame blocks until a decoded frame arrives or ctx is cancelled.
func (a *Adapter) RecvFrame(ctx context.Context) ([]byte, error) {
	select {
	case frame := <-a.modem.RXChan():
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Adapter) Status(_ context.Context) (types.DeviceStatus, error) {
	return types.Connected(), nil
}

func (a *Adapter) Close() error {
	_ = a.capDev.Stop()
	_ = a.playDev.Stop()
	a.capDev.Uninit()
	a.playDev.Uninit()
	_ = a.ctx.Uninit()
	a.modem.Close()
	return a.ptt.Close()
}

// ── helpers ─────────────────────────────────────────────────────────────────

// findDevice returns the malgo DeviceID for the first device whose name
// contains substr (case-insensitive). Returns nil for default device.
func findDevice(ctx *malgo.AllocatedContext, kind malgo.DeviceType, substr string) *malgo.DeviceID {
	if substr == "" {
		return nil
	}
	devs, err := ctx.Devices(kind)
	if err != nil {
		return nil
	}
	needle := strings.ToLower(substr)
	for _, d := range devs {
		if strings.Contains(strings.ToLower(d.Name()), needle) {
			id := d.ID
			return &id
		}
	}
	return nil
}
