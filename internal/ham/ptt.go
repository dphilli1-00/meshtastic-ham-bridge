package ham

// PTT controls push-to-talk on a radio.
type PTT interface {
	On() error
	Off() error
	Close() error
}

// VOXPTT is a no-op PTT — the radio keys automatically when it detects audio.
// Use on iOS or any setup where CM108 HID is unavailable.
type VOXPTT struct{}

func (VOXPTT) On() error    { return nil }
func (VOXPTT) Off() error   { return nil }
func (VOXPTT) Close() error { return nil }
