package main

import (
	"strings"
	"testing"
)

// Covers the `key` command — remote keyboard injection. Guests read
// the matrix through keyboard.Scan(addr), whose HIGH byte selects the
// scanned rows: addrHi=0 strobes every row (the all-rows form
// cruxay-style pollers use, and matches the emulator's IN model).
// A pressed column bit reads 0.

func TestCmdKey_DownUpRoundTrip(t *testing.T) {
	d := newRemoteWithCPU(t)
	kbd := d.emu.kbd
	if kbd == nil {
		t.Fatal("fixture lost its keyboard")
	}
	if got := d.handleCommand("key enter down"); got != "OK key down enter" {
		t.Fatalf("down = %q", got)
	}
	if v := kbd.Scan(0x00FE); v&0x01 != 0 {
		t.Fatalf("enter not visible in Scan: $%02X", v)
	}
	if got := d.handleCommand("key enter up"); !strings.HasPrefix(got, "OK key up") {
		t.Fatalf("up = %q", got)
	}
	if v := kbd.Scan(0x00FE); v&0x01 == 0 {
		t.Fatalf("enter still pressed after up: $%02X", v)
	}
}

func TestCmdKey_RawRowMask(t *testing.T) {
	d := newRemoteWithCPU(t)
	if got := d.handleCommand("key 0 0x10 down"); !strings.HasPrefix(got, "OK key down 0") {
		t.Fatalf("raw down = %q", got)
	}
	// Row 0 bit 4 = 'V' — with every row scanned, bit 4 reads low.
	if v := d.emu.kbd.Scan(0x00FE); v&0x10 != 0 {
		t.Fatalf("row0 mask$10 not pressed: $%02X", v)
	}
	// Row-scoped scan must NOT leak the press into row-6-only reads.
	if v := d.emu.kbd.Scan(0xFFFE); v&0x10 != 0x10 {
		t.Fatalf("press leaked into row-scoped scan (addrHi=$FF excludes row 0): $%02X", v)
	}
	if got := d.handleCommand("key 0 $10 up"); !strings.HasPrefix(got, "OK key up 0") {
		t.Fatalf("raw up = %q", got)
	}
	if v := d.emu.kbd.Scan(0x00FE); v&0x10 == 0 {
		t.Fatalf("bit still pressed after up: $%02X", v)
	}
}

func TestCmdKey_TapFrameCountdown(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.paused.Store(true) // command dispatch only; no frame loop running
	if got := d.handleCommand("key enter tap 3"); !strings.Contains(got, "tap enter 3f") || !strings.Contains(got, "paused") {
		t.Fatalf("tap = %q (paused machine must report the hold)", got)
	}
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 != 0 {
		t.Fatalf("tap did not press: $%02X", v)
	}
	d.TapTick() // frame 1
	d.TapTick() // frame 2
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 != 0 {
		t.Fatalf("released early at tick 2: $%02X", v)
	}
	d.TapTick() // frame 3 — countdown done
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 == 0 {
		t.Fatalf("tap not released after 3 frames: $%02X", v)
	}
	// Ringing again must not resurrect the key.
	d.TapTick()
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 == 0 {
		t.Fatalf("extra tick re-pressed/released: $%02X", v)
	}
}

func TestCmdKey_ChordTapsBothBits(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.paused.Store(true)
	// caps+space = BREAK: rows 0/7 bit 0 both fall together.
	if got := d.handleCommand("key caps+space tap 2"); !strings.HasPrefix(got, "OK key tap caps+space 2f") {
		t.Fatalf("chord tap = %q", got)
	}
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 != 0 {
		t.Fatalf("BREAK bit not pressed (bit0 must read low): $%02X", v)
	}
	d.TapTick()
	if v := d.emu.kbd.Scan(0x00FE); v&0x01 != 0 {
		t.Fatalf("BREAK released early at tick 1: $%02X", v)
	}
	d.TapTick()
	// After release, BREAK is up but a plain row scan is all-high again.
	if v := d.emu.kbd.Scan(0x00FE); v != 0xFF {
		t.Fatalf("chord residue after release: $%02X", v)
	}
}

func TestCmdKey_UpCancelsPendingTap(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.paused.Store(true)
	if got := d.handleCommand("key enter tap 50"); !strings.HasPrefix(got, "OK key tap enter 50f") {
		t.Fatalf("tap = %q", got)
	}
	if got := d.handleCommand("key enter up"); !strings.HasPrefix(got, "OK key up") {
		t.Fatalf("up = %q", got)
	}
	for i := 0; i < 60; i++ {
		d.TapTick()
	}
	if v := d.emu.kbd.Scan(0x00FE); v != 0xFF {
		t.Fatalf("cancelled tap re-released into the matrix: $%02X", v)
	}
}

func TestCmdKey_ReleaseAll(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.handleCommand("key enter down")
	d.handleCommand("key 3 0x0F down")
	if got := d.handleCommand("key release"); got != "OK keys released" {
		t.Fatalf("release = %q", got)
	}
	if v := d.emu.kbd.Scan(0x00FE); v != 0xFF {
		t.Fatalf("keys stuck after release: $%02X", v)
	}
}

func TestCmdKey_Errors(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.paused.Store(true)
	cases := []struct{ line, wantSub string }{
		{"key", "usage"},
		{"key boguskey down", "unknown key"},
		{"key 9 0x01 down", "ROW must be"},
		{"key 6 0xFF down", "MASK must be"},
		{"key enter tap 3 extra", "usage"},
		{"key enter down foo", "FRAMES"},
		{"key enter tap 0", "FRAMES"},
		{"key enter tap 99999", "FRAMES"},
	}
	for _, c := range cases {
		if got := d.handleCommand(c.line); !strings.HasPrefix(got, "ERR") || !strings.Contains(got, c.wantSub) {
			t.Errorf("handleCommand(%q) = %q, want ERR containing %q", c.line, got, c.wantSub)
		}
	}
}

func TestCmdKey_HelpMentionsKey(t *testing.T) {
	d := newRemoteWithCPU(t)
	d.paused.Store(true)
	if !strings.Contains(d.handleCommand("help"), " key ") {
		t.Fatal("help does not advertise `key`")
	}
}
