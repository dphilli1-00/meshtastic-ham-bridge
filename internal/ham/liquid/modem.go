//go:build liquid

package liquid

// #cgo LDFLAGS: -lliquid -lm
// #cgo linux CFLAGS: -I/usr/local/include
// #cgo linux LDFLAGS: -L/usr/local/lib
// #cgo darwin CFLAGS: -I/usr/local/include -I/opt/homebrew/include
// #cgo darwin LDFLAGS: -L/usr/local/lib -L/opt/homebrew/lib
// #include "liquid_shim.h"
// #include <stdlib.h>
import "C"

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// OFDM parameters. 64 subcarriers × 16-sample CP gives ~95 Hz/subcarrier
// across a 6 kHz bandwidth; trimmed to 300–2700 Hz band by the radio.
// For HF SSB, reduce to 32 subcarriers to fit 3 kHz passband.
const (
	DefaultSubcarriers = 64
	DefaultCPLen       = 16
	DefaultTaperLen    = 4

	// MaxTXSamples is the sample buffer handed to the C shim.
	// (subcarriers + cpLen) * ~200 symbols * 2 (I+Q) = generous upper bound.
	MaxTXSamples = 131072
)

// modemRegistry maps integer IDs to live Modem instances so the C callback
// can reach the right Go struct.
var (
	modemMu       sync.RWMutex
	modemRegistry = map[int]*Modem{}
	nextID        atomic.Int64
)

func registerModem(m *Modem) int {
	id := int(nextID.Add(1))
	modemMu.Lock()
	modemRegistry[id] = m
	modemMu.Unlock()
	return id
}

func unregisterModem(id int) {
	modemMu.Lock()
	delete(modemRegistry, id)
	modemMu.Unlock()
}

// goFrameCallback is called from C (via liquid_frame_callback in the shim)
// when a valid frame is decoded. Exported so CGO can see it.
//
//export goFrameCallback
func goFrameCallback(id C.int, payload *C.uint8_t, payloadLen C.uint32_t) {
	if payload == nil || payloadLen == 0 {
		return
	}
	modemMu.RLock()
	m, ok := modemRegistry[int(id)]
	modemMu.RUnlock()
	if !ok {
		return
	}
	// Copy payload before returning to C.
	data := C.GoBytes(unsafe.Pointer(payload), C.int(payloadLen))
	select {
	case m.rxCh <- data:
	default:
		// RX channel full — drop frame rather than block C callback.
	}
}

// Modem wraps liquid-dsp ofdmflexframegen + ofdmflexframesync.
// It encodes/decodes raw []byte frames to/from IQ sample streams.
// Audio I/O is handled by the parent Adapter (adapter.go).
type Modem struct {
	id   int
	cm   *C.liquid_modem_t // C-side state

	// rxCh receives decoded payloads from the C callback goroutine.
	rxCh chan []byte

	subcarriers uint
	cpLen       uint
	taperLen    uint
}

// NewModem creates a new OFDM flex frame modem with the given parameters.
// Use DefaultSubcarriers/CPLen/TaperLen for a sensible starting point.
func NewModem(subcarriers, cpLen, taperLen uint) (*Modem, error) {
	m := &Modem{
		rxCh:        make(chan []byte, 64),
		subcarriers: subcarriers,
		cpLen:       cpLen,
		taperLen:    taperLen,
	}
	m.id = registerModem(m)

	m.cm = C.liquid_modem_create(
		C.int(m.id),
		C.uint(subcarriers),
		C.uint(cpLen),
		C.uint(taperLen),
	)
	if m.cm == nil {
		unregisterModem(m.id)
		return nil, fmt.Errorf("liquid_modem_create failed (OOM or liquid-dsp not available)")
	}
	return m, nil
}

// TXSamples encodes payload into interleaved IQ float32 samples.
// Returns [real0, imag0, real1, imag1, ...].
// The caller (Adapter) converts these to the audio device's sample format
// and plays them through the radio. For FM radios, the IQ stream is down-
// mixed to a single audio carrier: audio = real * cos(2πf_c*t) - imag * sin(2πf_c*t).
func (m *Modem) TXSamples(payload []byte) ([]float32, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	cbuf := (*C.float)(C.malloc(C.size_t(MaxTXSamples * C.sizeof_float)))
	if cbuf == nil {
		return nil, fmt.Errorf("malloc failed")
	}
	defer C.free(unsafe.Pointer(cbuf))

	cp := C.CBytes(payload)
	defer C.free(cp)

	n := C.liquid_modem_tx_frame(
		m.cm,
		(*C.uint8_t)(cp),
		C.uint(len(payload)),
		cbuf,
		C.uint(MaxTXSamples),
	)
	if n == 0 {
		return nil, fmt.Errorf("liquid_modem_tx_frame returned 0 samples")
	}

	// Convert C float array → Go []float32.
	count := int(n) * 2 // n complex samples → 2n floats (I+Q interleaved)
	out := make([]float32, count)
	cslice := unsafe.Slice((*C.float)(cbuf), count)
	for i, v := range cslice {
		out[i] = float32(v)
	}
	return out, nil
}

// PushRXSamples feeds received interleaved IQ float32 samples to the
// synchroniser. Decoded frames arrive on RXChan().
// This is the hot path — called from the malgo audio capture callback.
func (m *Modem) PushRXSamples(samples []float32) {
	if len(samples) == 0 {
		return
	}
	cp := (*C.float)(C.malloc(C.size_t(len(samples)) * C.sizeof_float))
	if cp == nil {
		return
	}
	defer C.free(unsafe.Pointer(cp))

	cslice := unsafe.Slice(cp, len(samples))
	for i, v := range samples {
		cslice[i] = C.float(v)
	}
	C.liquid_modem_rx_push(m.cm, cp, C.uint(len(samples)/2))
}

// RXChan returns the channel on which decoded frame payloads are delivered.
func (m *Modem) RXChan() <-chan []byte { return m.rxCh }

// Close frees C-side resources.
func (m *Modem) Close() {
	unregisterModem(m.id)
	if m.cm != nil {
		C.liquid_modem_destroy(m.cm)
		m.cm = nil
	}
}
