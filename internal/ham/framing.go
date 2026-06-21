package ham

import "fmt"

// Simple robust framing for the Bell 202 audio modem.
//
// Frame format:
//   [24 bits]  Preamble: alternating 1,0,1,0... — locks bit clock
//   [16 bits]  Sync word: 0xEB90 — breaks alternating, marks frame start
//   [16 bits]  Length: payload byte count, big-endian uint16
//   [N bytes]  Payload
//   [16 bits]  CRC-16/CCITT (poly 0x1021, init 0xFFFF) over length+payload
//
// No bit stuffing. No long runs of same tone. Designed for clock recovery:
// every preamble bit has a tone transition, making sync trivial.
// After the preamble, the sync word's non-alternating pattern is distinctive.

const (
	frameSyncWord     = uint16(0xEB90) // sync word; must not be 0xAAAA or 0x5555
	framePreambleBits = 240            // ~200ms — survives radio key-up + audio buffer latency
	framePostambleBits = 60            // ~50ms — prevents CRC tail cutoff from early PTT drop
	frameMinPayload   = 1
	frameMaxPayload   = 512
)

// frameEncode encodes a payload into a sequence of bits for the modulator.
// true = mark (1200 Hz), false = space (2200 Hz).
func frameEncode(payload []byte) []bool {
	var bits []bool

	// Preamble: alternating 1,0,1,0,...
	for i := 0; i < framePreambleBits; i++ {
		bits = append(bits, i%2 == 0)
	}

	// Sync word: 0xEB90, MSB first.
	bits = appendUint16Bits(bits, frameSyncWord)

	// Length: big-endian uint16.
	bits = appendUint16Bits(bits, uint16(len(payload)))

	// Payload bytes, MSB first.
	for _, b := range payload {
		for i := 7; i >= 0; i-- {
			bits = append(bits, (b>>uint(i))&1 == 1)
		}
	}

	// CRC-16/CCITT over length field + payload.
	crc := crc16(uint16(len(payload)), payload)
	bits = appendUint16Bits(bits, crc)

	// Post-amble: alternating bits so the audio device flushes the CRC
	// before PTT drops. Prevents tail cutoff from audio buffer latency.
	for i := 0; i < framePostambleBits; i++ {
		bits = append(bits, i%2 == 0)
	}

	return bits
}

func appendUint16Bits(bits []bool, v uint16) []bool {
	for i := 15; i >= 0; i-- {
		bits = append(bits, (v>>uint(i))&1 == 1)
	}
	return bits
}

// crc16 computes CRC-16/CCITT (poly 0x1021, init 0xFFFF) over length+payload.
func crc16(length uint16, payload []byte) uint16 {
	crc := uint16(0xFFFF)
	feed := func(b byte) {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	feed(byte(length >> 8))
	feed(byte(length))
	for _, b := range payload {
		feed(b)
	}
	return crc
}

// ---------------------------------------------------------------------------
// Frame receiver state machine
// ---------------------------------------------------------------------------

type frameRxState int

const (
	stateHunt    frameRxState = iota // counting preamble alternating bits
	stateSync                        // preamble locked, scanning for sync word
	stateLength                      // reading 16-bit length
	stateData                        // reading payload bytes
	stateCRC                         // reading 16-bit CRC
)

type frameDecoder struct {
	state    frameRxState
	shiftReg uint16 // 16-bit shift register — holds last 16 bits for sync scan
	altCount int    // consecutive alternating bits seen
	prevBit  bool   // previous bit for alternating detection

	lenBits  int
	lenVal   uint16

	dataBuf  []byte
	dataIdx  int
	dataByte byte
	dataBit  int

	crcBits int
	crcVal  uint16
}

func newFrameDecoder() *frameDecoder { return &frameDecoder{} }

// push feeds one bit into the frame decoder.
// Returns a complete payload if a valid CRC-verified frame was received.
func (d *frameDecoder) push(bit bool) []byte {
	switch d.state {
	case stateHunt:
		return d.huntPush(bit)
	case stateSync:
		return d.syncPush(bit)
	case stateLength:
		return d.lengthPush(bit)
	case stateData:
		return d.dataPush(bit)
	case stateCRC:
		return d.crcPush(bit)
	}
	return nil
}

// huntPush counts alternating bits. Once we have enough, switch to sync scan.
func (d *frameDecoder) huntPush(bit bool) []byte {
	if bit != d.prevBit {
		d.altCount++
	} else {
		if d.altCount > 4 {
			fmt.Printf("[framing] preamble broken at altCount=%d\n", d.altCount)
		}
		d.altCount = 0 // non-alternating — restart
	}
	d.prevBit = bit
	if d.altCount >= framePreambleBits {
		fmt.Printf("[framing] preamble locked (%d alt bits) → sync scan\n", d.altCount)
		d.state = stateSync
		d.shiftReg = 0
		d.altCount = 0
	}
	return nil
}

// syncPush shifts bits looking for the sync word.
func (d *frameDecoder) syncPush(bit bool) []byte {
	var b uint16
	if bit {
		b = 1
	}
	d.shiftReg = d.shiftReg<<1 | b
	if d.shiftReg == frameSyncWord {
		fmt.Printf("[framing] SYNC found → length decode\n")
		d.state = stateLength
		d.lenBits = 0
		d.lenVal = 0
	}
	d.altCount++
	if d.altCount > 48 {
		fmt.Printf("[framing] sync timeout (shiftReg=0x%04X) → hunt\n", d.shiftReg)
		d.reset()
	}
	return nil
}

func (d *frameDecoder) lengthPush(bit bool) []byte {
	var b uint16
	if bit {
		b = 1
	}
	d.lenVal = d.lenVal<<1 | b
	d.lenBits++
	if d.lenBits == 16 {
		if d.lenVal == 0 || d.lenVal > frameMaxPayload {
			// Invalid length — back to hunt.
			d.reset()
			return nil
		}
		d.state = stateData
		d.dataBuf = make([]byte, d.lenVal)
		d.dataIdx = 0
		d.dataByte = 0
		d.dataBit = 0
	}
	return nil
}

func (d *frameDecoder) dataPush(bit bool) []byte {
	if bit {
		d.dataByte |= 1 << uint(7-d.dataBit)
	}
	d.dataBit++
	if d.dataBit == 8 {
		d.dataBuf[d.dataIdx] = d.dataByte
		d.dataIdx++
		d.dataByte = 0
		d.dataBit = 0
		if d.dataIdx == int(d.lenVal) {
			d.state = stateCRC
			d.crcBits = 0
			d.crcVal = 0
		}
	}
	return nil
}

func (d *frameDecoder) crcPush(bit bool) []byte {
	var b uint16
	if bit {
		b = 1
	}
	d.crcVal = d.crcVal<<1 | b
	d.crcBits++
	if d.crcBits == 16 {
		expected := crc16(d.lenVal, d.dataBuf)
		payload := d.dataBuf
		d.reset()
		if d.crcVal == expected {
			return payload
		}
		// CRC mismatch — discard and hunt again.
		return nil
	}
	return nil
}

func (d *frameDecoder) reset() {
	d.state = stateHunt
	d.shiftReg = 0
	d.altCount = 0
	d.lenBits = 0
	d.lenVal = 0
	d.dataBuf = nil
	d.dataIdx = 0
	d.dataByte = 0
	d.dataBit = 0
	d.crcBits = 0
	d.crcVal = 0
}
