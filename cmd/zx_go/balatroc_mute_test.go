package main

// balatroc_mute_test.go — MUTE gate (pass 48g rebasing onto the honest
// crt0 boot): M key (half-row 7, bit 0x04) toggles _mute in the LIVE game
// poll loop (input.c scanKey, line ~186). Proof ladder, in live game after
// a REAL START stroke (music_boot is armed by _main BEFORE the title, so
// the heartbeat beats from frame one):
//   1. live game unmuted: AY mirror _ayc[7] == $3C (MIX_IDLE)
//   2. M stroke        : _mute 0->1, mixer rewrites to $3F (MIX_MUTE)
//   3. heartbeat alive THROUGH mute (quarks keep counting)
//   4. arm an SFX while muted (_sfx_id=5 + a safe vol-0 row): the mixer
//      opens MIX_SFXC $3B — chC stays sovereign even muted (pass 48e law)
//   5. M again         : _mute 1->0, mixer returns to $3C
// Harness constants from board.map of tap c9827c58…:
// _mute=$0xfd24 _qctr=$0xfd1d _sfx_id=$0xfd1f _sfxp=$0xfd20 _ayc=$0xfd2b.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocMutePlus2(t *testing.T) {
	if os.Getenv("MUTEK") == "" {
		t.Skip("set MUTEK=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = 0x8000 // crt0 — the honest entry (title polls keys)
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row int, bit byte, tail int) {
		emu.kbd.PressMatrixKey(row, bit, true)
		runF(2)
		emu.kbd.PressMatrixKey(row, bit, false)
		runF(tail)
	}
	rd8 := func(ad uint16) int { return int(emu.mem.Read(ad)) }
	q := func() int { return int(emu.mem.Read(0xFD1D)) }
	ay7 := func() int { return rd8(0xfd2b + 7) }
	// honest entry: crt0 -> _main -> music_boot -> title (polls START)
	runF(160)
	if ay7() != 0x3C {
		t.Fatalf("title mixer not MIX_IDLE: ay7=$%02X", ay7())
	}
	stroke(4, 0x01, 140) // START into the live game
	if ay := ay7(); ay != 0x3C && ay != 0x38 {
		t.Fatalf("game mixer not unmuted: ay7=$%02X", ay)
	}
	if rd8(0xfd24) != 0 {
		t.Fatalf("mute starts %d, want 0", rd8(0xfd24))
	}
	// M down -> mute=1; mixer MUST rewrite to MIX_MUTE $3F
	stroke(7, 0x04, 40)
	m1 := rd8(0xfd24)
	q1 := q()
	runF(120)
	q2 := q()
	// arm an SFX WHILE muted: chC sovereignty => mixer opens $3B
	emu.mem.Write(0xfd20, 0x10)
	emu.mem.Write(0xFCD8, 0x7D) // _sfxp HI = $7D10 (free RAM row)
	emu.mem.Write(0x7D10, 0x00) // tone lo
	emu.mem.Write(0x7D11, 0x00) // tone hi
	emu.mem.Write(0x7D12, 0x00) // vol 0 -> plays, then auto-disarm
	emu.mem.Write(0xfd1f, 5)    // _sfx_id = 5
	saw3B := false
	for i := 0; i < 8; i++ {
		runF(1)
		if ay7() == 0x3B {
			saw3B = true
		}
	}
	// M again -> unmute; mixer must close back to MIX_IDLE $3C
	stroke(7, 0x04, 60)
	m2 := rd8(0xfd24)
	fmt.Printf("mute: %d->%d, quark %d->%d while muted, ay7=%02X sawSFXC=%v\n",
		m1, m2, q1, q2, ay7(), saw3B)
	if m1 != 1 || m2 != 0 {
		t.Fatalf("M-key mute toggle dead: %d %d", m1, m2)
	}
	if q2 <= q1+40 {
		t.Fatalf("heartbeat stalled during mute: %d->%d over 120 frames", q1, q2)
	}
	if !saw3B {
		t.Fatalf("chC sovereignty dead while muted: mixer never opened MIX_SFXC")
	}
	if ay7() != 0x3C {
		t.Fatalf("unmute left mixer at $%02X, want $3C", ay7())
	}
}
