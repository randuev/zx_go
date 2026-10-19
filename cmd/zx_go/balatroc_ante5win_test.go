package main

// balatroc_ante5win_test.go — SEVA shots 38/39: at ANTE 5 a SINGLE card
// play printed "BLIND DEFEATED!" — the live target died (tgt/=10 to zero)
// while HUD digits survived in the buffer. LAW: after compute_target at
// any state, tgt must equal the printed number.
//
// PASS-48g REBASE (honest-path law): _main is built sdcc_iy — EVERY local
// is (iy+off), so jumping PC straight into _main/$0xab4d without crt0's IY
// frame leaves the state machine reading garbage (the old harness "worked"
// only while the poll player papered over it). This test now boots through
// crt0 ($8000) exactly like the tape loader's RANDOMIZE USR, presses START,
// and climbs to ante 5 with REAL strokes only:
//   per round: poke tgt=1 (any scoring hand wins), SEL one card, PLAY
//   (score_anim -> score_wait key -> win banner key -> payout -> SHOP ->
//   PLAY exits -> next_round -> compute_target runs FOR REAL).
// Then the ante-5 attack: live tgt must be 11250 (small blind, pass 35/47
// ladder), a tiny 1440-score play must NOT announce STR_WIN.
// Constants: board.map of tap c9827c58… (_tgt=$0xfca7 _score=$0xfedf
// _ante=$0xfca5 _blind=$FEE4 _sfx_id=$0xfcd6, _STR_WIN=$0xdb35 banner id check
// via sfx channel: sfx_win uses id 7 -> _sfx_id).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocAnte5WinPlus2(t *testing.T) {
	if os.Getenv("ANTE5") == "" {
		t.Skip("set ANTE5=1")
	}
	tapRaw, _ := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	blocks := balParseTapBlocks(t, tapRaw)
	var a uint16
	var bd, md []byte
	for i := range blocks {
		b := blocks[i]
		if b.hdr && b.typ == 3 && trimName(b.name) == "board" && i+1 < len(blocks) {
			a, bd = b.start, blocks[i+1].data
		}
		if b.hdr && b.typ == 3 && trimName(b.name) == "music" && i+1 < len(blocks) {
			md = blocks[i+1].data
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
	const ante = 0xfca5
	const blind = 0xFEE4
	const tgt = 0xfca7
	const score = 0xfedf
	const handsL = 0xfca3
	const sfxid = 0xfcd6 // pass48c: wins announce as chC jingle id 7
	const STRWIN = 7
	rd16 := func(addr uint16) uint32 {
		return uint32(emu.mem.Read(addr)) | uint32(emu.mem.Read(addr+1))<<8 |
			uint32(emu.mem.Read(addr+2))<<16 | uint32(emu.mem.Read(addr+3))<<24
	}
	wu32 := func(addr uint16, v uint32) {
		for j := 0; j < 4; j++ {
			emu.mem.Write(addr+uint16(j), byte(v>>uint(8*j)))
		}
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range bd {
		emu.mem.Write(a+uint16(i), v)
	}
	if md != nil {
		for i, v := range md {
			emu.mem.Write(0x6000+uint16(i), v)
		}
	}
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = 0x8000 // crt0 — the ONLY honest door
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	stroke := func(row byte, bit byte, tail int) {
		emu.kbd.PressMatrixKey(int(row), bit, true)
		runF(4)
		emu.kbd.PressMatrixKey(int(row), bit, false)
		runF(tail)
	}
	runF(160) // crt0 -> music_boot -> title poll
	stroke(4, 0x01, 200) // START -> game_new -> round 1
	for rnd := 0; rnd < 40 && emu.mem.Read(ante) < 5; rnd++ {
		runF(140) // deal poll settle
		wu32(tgt, 1) // any scoring hand beats this round
		stroke(4, 0x08, 60) // SEL a card
		stroke(4, 0x01, 700) // PLAY -> score_anim -> wait_key
		runF(240)            // win banner + payout + shop paint
		stroke(4, 0x01, 700) // leave shop: NEXT round (compute_target REAL)
		fmt.Printf("rnd %2d: ante=%d blind=%d tgt=%d hands_l=%d PC=$%04X\n",
			rnd, emu.mem.Read(ante), emu.mem.Read(blind), rd16(tgt),
			emu.mem.Read(handsL), emu.cpu.PC)
	}
	anteNow := emu.mem.Read(ante)
	blindNow := emu.mem.Read(blind)
	tg := rd16(tgt)
	fmt.Printf("ante5 arrival: ante=%d blind=%d tgt=%d\n", anteNow, blindNow, tg)
	if anteNow != 5 {
		t.Fatalf("win-climb stalled at ante=%d (want 5)", anteNow)
	}
	if tg != 11250 {
		t.Fatalf("ante-5 small target wrong via real flow: tgt=%d want 11250", tg)
	}
	// full play path at ante 5: score 1440 < tgt 11250 must NOT announce
	// STR_WIN (the pass-46d lie was tgt destroyed to 0).
	wu32(score, 1440)
	stroke(4, 0x08, 40) // SEL
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	winLie := false
	for f := 0; f < 900; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		m := emu.mem.Read(sfxid)
		if m == STRWIN {
			fmt.Printf("!! STR_WIN at frame %d: score=%d tgt=%d\n", f, rd16(score), rd16(tgt))
			winLie = rd16(score) < rd16(tgt)
			break
		}
	}
	fmt.Printf("post-play ante5: score=%d tgt=%d hands_l=%d PC=$%04X\n",
		rd16(score), rd16(tgt), emu.mem.Read(handsL), emu.cpu.PC)
	if winLie {
		t.Fatalf("FALSE WIN at ante5: score=%d tgt=%d", rd16(score), rd16(tgt))
	}
	if tg := rd16(tgt); tg == 0 {
		t.Fatalf("tgt destroyed to 0 again: %d", tg)
	}
	fmt.Println("ante5 target lives; no false win")
}
