package main

// balatroc_shop_help_test.go — shared harness helper, PASS-48g HONEST-PATH
// LAW: every shop gate must arrive at the shop the way the player does —
// crt0 ($8000) boots _main, START enters round 1, tgt is poked to 1 so any
// scoring hand wins, PLAY runs the REAL score_anim -> score_wait ->
// win_payout -> banner -> SHOP pipeline. No direct PC jumps into
// shop_screen: it is compiled sdcc_iy and its return path into a corrupted
// caller frame restarts the machine (Seva's "just restarts to basic").
// Shop entry is confirmed by sampling PC inside the shop span (the shop
// repaint+poll lives there); paint and offer ids prove it further.
// Constants: board.map of tap c9827c58… (_tgt=$0xfca7 _shop_id=$0xff17
// _shop_cost=$0xff19 _shop_sold=$0xff27 _shop_cur=$0xff29 _bank=$0xfcb7 _qctr=$0xfcd4).

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func balBootLive(t *testing.T) *emulator {
	t.Helper()
	tapRaw, err := os.ReadFile("/root/nerve-workspace/demos/balatroc-c/build/board.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
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
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator: %v", err)
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
	return emu
}

// balStroke is the harness key-stroke: press, settle, release, settle.
func balStroke(emu *emulator, runF func(int), row byte, bit byte, tail int) {
	emu.kbd.PressMatrixKey(int(row), bit, true)
	runF(4)
	emu.kbd.PressMatrixKey(int(row), bit, false)
	runF(tail)
}

// balToShop climbs to the FIRST shop over the REAL win pipeline. Each
// attempt pokes tgt=1 (any scoring hand wins), SELs the cursor card and
// re-strokes PLAY every ~150 frames (strokes that land while the loop is
// busy are dropped, so we keep knocking). WIN evidence is the PAYOUT: bank
// strictly grows (win_payout runs only on the win->shop path). The moment
// bank moves we exit the win banner with one ENTER and the shop is live.
func balToShop(t *testing.T, rounds int) *emulator {
	t.Helper()
	emu := balBootLive(t)
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	rd := func(ad uint16) uint32 {
		return uint32(emu.mem.Read(ad)) | uint32(emu.mem.Read(ad+1))<<8 |
			uint32(emu.mem.Read(ad+2))<<16 | uint32(emu.mem.Read(ad+3))<<24
	}
	runF(160) // crt0 -> music_boot -> title poll
	balStroke(emu, runF, 4, 0x01, 200) // START -> round 1
	for r := 0; r < rounds; r++ {
		runF(140) // deal + compute_target settle
		for j := 0; j < 4; j++ { // poke tgt=1 — any scoring hand wins
			emu.mem.Write(0xfca7+uint16(j), 0)
		}
		emu.mem.Write(0xfca7, 1)
		balStroke(emu, runF, 4, 0x08, 60) // SEL the cursor card
		bank0 := emu.mem.Read(0xfcb7)
		won := false
		balStroke(emu, runF, 4, 0x01, 150) // PLAY -> score_anim -> tally
		for i := 0; i < 900 && !won; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			if emu.mem.Read(0xfcb7) != bank0 {
				won = true // WIN PAYOUT banked — shop follows the banner
			}
			if i%150 == 149 {
				balStroke(emu, runF, 4, 0x01, 4) // knock again if busy
			}
		}
		if !won {
			fmt.Printf("shopretry %d: ante=%d hands_l=%d tgt=%d bank=%d PC=$%04X\n",
				r, emu.mem.Read(0xfca5), emu.mem.Read(0xfca3), rd(0xfca7),
				emu.mem.Read(0xfcb7), emu.cpu.PC)
			runF(400)
			continue
		}
		balStroke(emu, runF, 4, 0x01, 60) // exit win banner -> SHOP paints
		runF(60)
		return emu // shop is live: its own poll loop owns the machine
	}
	t.Fatalf("never reached shop through real win pipeline (%d rounds)", rounds)
	return nil
}
