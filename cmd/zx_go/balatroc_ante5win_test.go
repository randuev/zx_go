package main

// balatroc_ante5win_test.go — SEVA shots 38/39: at ANTE 5 a SINGLE card
// play printed "BLIND DEFEATED!" — the live target died (tgt/=10 to zero)
// while HUD digits survived in the buffer. LAW: after compute_target at
// any state, tgt must equal the printed number.
//
// PASS-48g REBASE (honest-path law): _main is built sdcc_iy — EVERY local
// is (iy+off), so jumping PC straight into _main/$0xaba7 without crt0's IY
// frame leaves the state machine reading garbage (the old harness "worked"
// only while the poll player papered over it). This test now boots through
// crt0 ($8000) exactly like the tape loader's RANDOMIZE USR, presses START,
// and climbs to ante 5 with REAL strokes only:
//   per round: poke tgt=1 (any scoring hand wins), SEL one card, PLAY
//   (score_anim -> score_wait key -> win banner key -> payout -> SHOP ->
//   PLAY exits -> next_round -> compute_target runs FOR REAL).
// Then the ante-5 attack: live tgt must be 11250 (small blind, pass 35/47
// ladder), a tiny 1440-score play must NOT announce STR_WIN.
// Addresses live: rebased from build/board.map via balSym() at run time.
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
		if b.hdr && b.typ == 3 && trimName(b.name) == "core" && i+1 < len(blocks) {
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
	ante := balSym("ante")
	blind := balSym("blind")
	tgt := balSym("tgt")
	score := balSym("score")
	handsL := balSym("hands_l")
	sfxid := balSym("sfx_id") // pass48c: wins announce as chC jingle id 7
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
	// PASS 56 RING: preload pages through the MMU window (door law)
	balRingPreload(t, emu, tapRaw)

	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xBFF5
	emu.mem.Write(0xBFF5, byte(sent))
	emu.mem.Write(0xBFF6, byte(sent>>8))
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
	spW := emu.cpu.SP
	for i := 0; i < 160; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.SP != spW {
			// single-step back is not possible; instead hunt for LD SP,HL ($F9)
			// and LD SP,(nn) ($ED $7B) in the resident code block
			for j := uint16(0x8000); j < 0x8600; j++ {
				if emu.mem.Read(j) == 0xF9 {
					fmt.Printf("LD SP,HL @$%04X (ctx:", j)
					for k := j - 6; k < j+2; k++ {
						fmt.Printf(" %02X", emu.mem.Read(k))
					}
					fmt.Printf(")\n")
				}
				if emu.mem.Read(j) == 0xED && emu.mem.Read(j+1) == 0x7B {
					nn := uint16(emu.mem.Read(j+2)) | uint16(emu.mem.Read(j+3))<<8
					fmt.Printf("LD SP,($%04X) @self=$%04X value=$%04X\n", nn, j, uint16(emu.mem.Read(nn))|uint16(emu.mem.Read(nn+1))<<8)
				}
			}
			break
		}
	}
	fmt.Printf("BOOT witness: PC=$%04X SP=$%04X IM=%d @8000=$%02X ante=$%02X qctr=%d\n",
		emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.mem.Read(0x8000), emu.mem.Read(ante), int(rd16(balSym("qctr"))))
	stroke(4, 0x01, 200) // START -> game_new -> round 1
	ring := make([]uint16, 0, 8192)
	type pcs struct{ p, s uint16 }
	ring2 := make([]pcs, 0, 200000)
	for rnd := 0; rnd < 40 && emu.mem.Read(ante) < 5; rnd++ {
		crashed := false
		for i := 0; i < 140 && !crashed; i++ {
			for j := 0; j < 70000; j++ {
				emu.cpu.StepInstructionWithIRQ()
				ring = append(ring, emu.cpu.PC)
				ring2 = append(ring2, pcs{emu.cpu.PC, emu.cpu.SP})
				if len(ring2) > 200000 {
					ring2 = ring2[len(ring2)-200000:]
				}
				if len(ring) > 8192 {
					ring = ring[len(ring)-8192:]
				}
				if emu.cpu.PC < 0x8000 && emu.cpu.PC != 0x7D00 {
					crashed = true
					break
				}
			}
		}
		if crashed {
			fmt.Printf("RING CRASH rnd=%d PC=$%04X SP=$%04X bank=$%02X flow=$%02X tail:\n",
				rnd, emu.cpu.PC, emu.cpu.SP,
				emu.mem.Read(balSym("bankbyte")), emu.mem.Read(balSym("flow")))
			for _, p := range ring[len(ring)-48:] {
				fmt.Printf(" %04X", p)
			}
			// find where SP starts its terminal descent
			fmt.Printf("\nSINK SCAN (first pc where SP<0xBF00 sustained):")
			for k := len(ring2) - 1; k > 0; k-- {
				if ring2[k].s >= 0xBF00 && ring2[k-1].s < 0xBF00 {
					lo := k - 6
					if lo < 0 {
						lo = 0
					}
					for _, e := range ring2[lo : k+1] {
						fmt.Printf(" %04X/$%04X", e.p, e.s)
					}
					break
				}
			}
			fmt.Printf("\n")
			fmt.Printf("\n")
			break
		}
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
