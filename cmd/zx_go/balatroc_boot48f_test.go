package main

// balatroc_boot48f_test.go — PASS 48f bring-up diagnostic: boot the tap,
// CALL _music_boot through the sentinel trampoline, then run frames and
// report PC/SP/IM/IFF/qctr every frame. Answers: does the IM2 player stay
// alive, where does PC go wrong, and does the main poll loop live-fire.
// Harness constants from board.map of tap 888f1d46…

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestBalatrocBoot48fPlus2(t *testing.T) {
	if os.Getenv("BOOT48F") == "" {
		t.Skip("set BOOT48F=1")
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
	if bd == nil {
		t.Fatal("no board block")
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
	const musicBoot = 0xD588
	const isr = 0xD5F9
	const qctr = 0xFCD1
	const vectab = 0x5B00
	emu.paused.Store(false)
	for i := 0; i < 200; i++ {
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
	// verify the image landed: code bytes at _music_boot
	fmt.Printf("board@%04X len=%d\n", a, len(bd))
	fmt.Printf("at music_boot: %02X %02X %02X %02X %02X\n",
		emu.mem.Read(musicBoot), emu.mem.Read(musicBoot+1), emu.mem.Read(musicBoot+2),
		emu.mem.Read(musicBoot+3), emu.mem.Read(musicBoot+4))
	sent := uint16(0x7D00)
	emu.mem.Write(sent, 0x76)
	emu.cpu.SP = 0xFF56
	emu.mem.Write(0xFF56, byte(sent))
	emu.mem.Write(0xFF57, byte(sent>>8))
	emu.cpu.PC = musicBoot
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	// single-step boot tracing PC/SP at every return-ish opcode
	for i := 0; i < 4000; i++ {
		if emu.cpu.Halted {
			emu.cpu.Halted = false
		}
		p0, sp0 := emu.cpu.PC, emu.cpu.SP
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == sent || emu.cpu.SP > 0xFF60 ||
			emu.cpu.PC < 0x8000 || emu.cpu.PC > 0xFFFF {
			fmt.Printf("step %d: PC $%04X->$%04X SP $%04X->$%04X (opcode $%02X)\n",
				i, p0, emu.cpu.PC, sp0, emu.cpu.SP, emu.mem.Read(p0))
			break
		}
	}
	fmt.Printf("after boot: PC=$%04X SP=$%04X IM=%d IFF1=%v I=$%02X Halted=%v\n",
		emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1, emu.cpu.I, emu.cpu.Halted)
	w := func(ad uint16) uint16 {
		return uint16(emu.mem.Read(ad)) | uint16(emu.mem.Read(ad+1))<<8
	}
	// ODD-FLOAT law: fetch addr = I<<8 | $ED (odd). The table must be stored
	// swapped — hi byte at even offsets, lo byte at odd — so the odd fetch
	// word reads _isr. Verify the exact fetch window AND both parity bytes.
	fmt.Printf("fetch float@%04X word=$%04X (want $%04X)\n", vectab|0xFF, w(vectab|0xFF), isr)
	badp := 0
	for i := 1; i < 0xFF; i += 2 { // words at ODD offsets: lo@odd, hi@even
		if emu.mem.Read(uint16(vectab+i)) != byte(isr&0xFF) ||
			emu.mem.Read(uint16(vectab+i+1)) != byte(isr>>8) {
			badp++
		}
	}
	if emu.mem.Read(0x5C00) != byte(isr>>8) { // 257th byte (hi for float $FF)
		badp++
	}
	fmt.Printf("vectab bad odd-offset words + 257th byte: %d\n", badp)
	// run frames; PC per frame
	q := func() int { return int(emu.mem.Read(qctr)) | int(emu.mem.Read(qctr+1))<<8 }
	// BOOT THROUGH THE REAL ENTRY POINT: $8000 crt0 — exactly what the
	// loader's RANDOMIZE USR 32768 does. No trampolines, no half-set frames.
	// crt0 runs DI, sets the SDCC IY frame, SP, IM and CALLs _main, which
	// calls _music_boot, paints the title, and runs the game forever.
	emu.cpu.PC = 0x8000
	emu.cpu.SP = 0xFF56
	emu.cpu.Halted = false
	runF := func(n int) {
		for i := 0; i < n; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}
	// title settles, then ENTER into the game (key_capture consumes it)
	// single-step the title: find the EXACT instruction that leaves the
	// game space (first vsync after IM2 is the suspect).
	var hist [20000]struct{ pc, sp uint16; iff bool }
	hn := 0
	step1 := func() {
		if emu.cpu.Halted {
			emu.cpu.Halted = false
		}
		emu.cpu.StepInstructionWithIRQ()
		hist[hn%700] = struct{ pc, sp uint16; iff bool }{emu.cpu.PC, emu.cpu.SP, emu.cpu.IFF1}
		hn++
	}
	crash := false
	for i := 0; i < 400000 && !crash; i++ {
		step1()
		if emu.cpu.PC < 0x8000 {
			fmt.Printf("ESCAPE at step %d PC=$%04X SP=$%04X IM=%d IFF1=%v I=$%02X\n",
				i, emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1, emu.cpu.I)
			fmt.Printf("cpu.IM2Vector=$%02X\n", emu.cpu.IM2Vector)
			fmt.Print("vectab raw $5BF0..$5C04: ")
			for ad := 0x5BF0; ad < 0x5C04; ad++ {
				fmt.Printf("%02X", emu.mem.Read(uint16(ad)))
			}
			fmt.Println()
			fmt.Println("last transitions (pc sp iff):")
			nn := hn
			if nn > 20000 {
				nn = 20000
			}
			for k := 0; k < nn; k++ {
				j := (hn - nn + k) % 20000
				h := hist[j]
				fmt.Printf("  $%04X $%04X %v\n", h.pc, h.sp, h.iff)
			}
			crash = true
		}
	}
	if !crash {
		fmt.Println("400K steps: title ALIVE, no escape")
	}
	if false {
	_ = 0
	fmt.Printf("title alive: PC=$%04X q=%d\n", emu.cpu.PC, q())
	}
	emu.kbd.PressMatrixKey(4, 0x01, true)
	runF(4)
	emu.kbd.PressMatrixKey(4, 0x01, false)
	for i := 0; i < 120; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.cpu.PC < 0x8000 {
			fmt.Printf("GAME-ESCAPE f%d PC=$%04X SP=$%04X IM=%d IFF1=%v\n",
				i, emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1)
			break
		}
	}
	q0 := q()
	for i := 0; i < 12; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		fmt.Printf("f%02d PC=$%04X SP=$%04X IM=%d IFF1=%v q=%d\n",
			i, emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1, q())
	}
	dq := q() - q0
	fmt.Printf("live-game quarks: %d over 12 frames (%.2f/frame)\n", dq, float64(dq)/12)
	if dq < 10 {
		t.Fatalf("music ISR NOT beating in live game: %d quarks/12f", dq)
	}
	// confirm every ODD-offset vectab word points at isr (odd-float law)
	bad := 0
	for v := 1; v < 0x100; v += 2 { // every ODD float offset must yield _isr
		if w(uint16(vectab|uint16(v))) != isr {
			bad++
		}
	}
	fmt.Printf("vectab bad odd-float entries: %d\n", bad)
}
