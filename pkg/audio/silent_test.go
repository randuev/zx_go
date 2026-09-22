package audio

import "testing"

// TestSilentSystemHasNoDevice verifies NewSilent produces a mixer that
// reports silent, never opens a player, and starts as a no-op. This is the
// mixer headless --record-audio relies on when no sound card exists: Start
// must return nil rather than fail, because the recording path pumps it
// directly.
func TestSilentSystemHasNoDevice(t *testing.T) {
	as := NewSilent()
	if !as.IsSilent() {
		t.Fatal("NewSilent must report IsSilent() == true")
	}
	if as.player != nil {
		t.Fatal("silent system must not attach an oto player")
	}
	if err := as.Start(); err != nil {
		t.Fatalf("Start on silent system must be a no-op, got %v", err)
	}
}

// TestSilentPumpDrainsOneFramePerCall verifies PumpFrames consumes exactly
// SamplesPerFrame slots per frame and delivers what was pushed. This is the
// pacing contract the headless loop depends on: one PumpFrames(1) per
// emulated frame keeps the WAV in lockstep with guest T-states.
func TestSilentPumpDrainsOneFramePerCall(t *testing.T) {
	as := NewSilent()
	frame := make([]int16, SamplesPerFrame*ChannelCount)
	for i := range frame {
		frame[i] = 1000
	}
	as.PushStereoSamples(frame)

	// Drain two frames: the first must return the pushed content, the
	// second the underrun decay tail (which holds last values or decays
	// toward zero depending on the queue policy — assert the first only).
	as.PumpFrames(1)
	buf := as.pumpBuf
	if len(buf) != SamplesPerFrame*ChannelCount*2 {
		t.Fatalf("pumpBuf %d bytes, want %d", len(buf), SamplesPerFrame*ChannelCount*2)
	}
	for i := 0; i < SamplesPerFrame*ChannelCount; i++ {
		s := int16(buf[i*2]) | int16(buf[i*2+1])<<8
		if s != 1000 {
			t.Fatalf("sample %d = %d, want 1000 (pumped frame must carry the pushed content)", i, s)
			break
		}
	}
}

// TestSilentResetEmptiesQueue verifies Reset on a silent mixer drains the
// queue instead of prefilling it — prefill would inject phantom silence
// ahead of a fresh frame and shift the recording timeline.
func TestSilentResetEmptiesQueue(t *testing.T) {
	as := NewSilent()
	frame := make([]int16, SamplesPerFrame*ChannelCount)
	for i := range frame {
		frame[i] = 500
	}
	as.PushStereoSamples(frame)
	if as.queueSize != SamplesPerFrame*ChannelCount {
		t.Fatalf("queueSize %d after push, want %d", as.queueSize, SamplesPerFrame*ChannelCount)
	}
	as.Reset()
	if as.queueSize != 0 {
		t.Fatalf("Reset on silent mixer must drain the queue, queueSize=%d", as.queueSize)
	}
}
