package main

// TestProbePlotPlus2 — ground truth for crux256 v3 engine design.
// Boots a real +2 core, pokes a tiny probe that CALLs PLOT-SUB ($22E5)
// three times with distinct coords/ink, halts the CPU immediately AFTER
// each call (via BreakpointCheck on the following NOPs), and dumps ALL
// registers + target screen byte + attr byte. This tells us exactly which
// registers survive the ROM plot routine — the engine must keep its loop
// state only in survivors (no push/pop budget).
//
// Run: go test ./cmd/zx_go/ -run TestProbePlotPlus2 -v

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestProbePlotPlus2(t *testing.T) {
	bin, err := os.ReadFile("/tmp/probe_plot.bin")
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	t.Logf("probe len=%d", len(bin))

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)

	// Boot to BASIC prompt like a real +2 before RANDOMIZE USR.
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("boot: ScreenPage=%d PC=$%04X SP=$%04X", emu.mem.ScreenPage, emu.cpu.PC, emu.cpu.SP)

	// Poke probe at $8000 (RAM slot, same as LOAD puts it).
	for i, b := range bin {
		emu.mem.Write(uint16(0x8000+i), b)
	}
	// verify first bytes
	for i := 0; i < 6; i++ {
		if emu.mem.Read(uint16(0x8000+i)) != bin[i] {
			t.Fatalf("poke mismatch at +%02X: got %02X want %02X", i, emu.mem.Read(uint16(0x8000+i)), bin[i])
		}
	}

	// Compute NOP-stop addresses from the known probe layout:
	// plot#1 CALL ends at $8000+1+5+3+3+3+3 = wait compute from disasm bytes:
	// 3E07 328F5C 3EF8 32905C AF 32915C 0664 0E28 CD E5 22 -> CALL at +$16? compute:
	// ld a,7:3E07(2) ld(5C8F):328F5C(3) ld a,F8:3EF8(2) ld(5C90):32905C(3)
	// xor:AF(1) ld(5C91):32915C(3) ld b,100:0664(2) ld c,40:0E28(2) CALL:CD E5 22(3)
	// => CALL at 0x8000+18? offsets: 2+3+2+3+1+3+2+2=18 -> CALL at +18, next insn +21
	// after NOPs x4 -> plot2 setup: ld b,175(2) ld c,127(2): +? easier: find CD E5 22 in bin:
	var callAddrs []uint16
	for i := 0; i < len(bin)-2; i++ {
		if bin[i] == 0xCD && bin[i+1] == 0xE5 && bin[i+2] == 0x22 {
			callAddrs = append(callAddrs, uint16(0x8000+i))
		}
	}
	if len(callAddrs) < 5 {
		t.Fatalf("expected 5 CALL $22E5, got %d", len(callAddrs))
	}
	t.Logf("CALL $22E5 at $%04X $%04X $%04X $%04X $%04X", callAddrs[0], callAddrs[1], callAddrs[2], callAddrs[3], callAddrs[4])

	// plot coords exactly as in probe_plot.zasm
	type probe struct{ y, x byte; desc string }
	probes := []probe{{100, 40, "mid"}, {175, 127, "topright"}, {0, 0, "bottomleft"},
		{10, 100, "steep-diag-start"}, {80, 33, "shallow-byte-cross"}}

	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000

	// BreakpointCheck: halt right after each CALL rets (PC == callAddr+3).
	nextStop := 0
	haltedAt := uint16(0)
	emu.cpu.BreakpointCheck = func(pc uint16) bool {
		if nextStop < len(callAddrs) && pc == callAddrs[nextStop]+3 {
			haltedAt = pc
			emu.paused.Store(true)
			return true
		}
		// ROM ($0000-$3FFF) and our code window are legitimate; anything
		// into $4000-$7FFF or past the probe block is an escape.
		if pc >= 0x4000 && pc <= 0x7FFF {
			t.Errorf("PC escaped to BASIC workspace: $%04X", pc)
			emu.paused.Store(true)
			return true
		}
		if pc > 0x8000+uint16(len(bin))+8 {
			t.Errorf("PC ran past probe: $%04X", pc)
			emu.paused.Store(true)
			return true
		}
		return false
	}

	regs := func() string {
		c := emu.cpu
		return fmt.Sprintf("AF=$%02X%02X BC=$%04X DE=$%04X HL=$%04X IX=$%04X IY=$%04X SP=$%04X",
			c.A, c.F, c.BC(), c.DE(), c.HL(), c.IX, c.IY, c.SP)
	}

	// pre-known target addresses via canonical ULA formula:
	ulaAddr := func(y, x byte) uint16 {
		y16 := uint16(y)
		return 0x4000 | ((y16 & 7) << 8) | ((y16 & 0x38) << 2) | ((y16 & 0xC0) << 5) | uint16(x/8)
	}
	attrAddr := func(y, x byte) uint16 {
		y16 := uint16(y)
		return 0x5800 | ((y16 & 0xC0) << 5) | ((y16 & 0x38) << 2) | uint16(x/8)
	}

	// log ALL RAM writes during plots (hook gives physical bank + addr)
	var wlog []string
	emu.mem.SetRAMWriteHook(func(bank int, addr uint16, val byte) {
		wlog = append(wlog, fmt.Sprintf("b%d $%04X=%02X pc=$%04X", bank, addr, val, emu.cpu.PC))
	})
	defer emu.mem.SetRAMWriteHook(nil)

	for i, ca := range callAddrs {
		_ = ca
		p := probes[i]
		wlog = nil
		// run until halt
		frames := 0
		for !emu.paused.Load() && frames < 50 {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			frames++
		}
		if !emu.paused.Load() {
			t.Fatalf("probe %d: never halted at $%04X", i, ca+3)
		}
		addr := ulaAddr(p.y, p.x)
		aa := attrAddr(p.y, p.x)
		bit := uint8(1 << (7 - p.x%8))
		pix := emu.mem.Read(addr)
		attr := emu.mem.Read(aa)
		t.Logf("PLOT %s (ROM y=%d x=%d): PC=$%04X halted_at=$%04X  %s", p.desc, p.y, p.x, emu.cpu.PC, haltedAt, regs())
		t.Logf("   screen $%04X=%02X (want bit %02X set:%v)  attr $%04X=%02X (ink=%02X want 07:%v)",
			addr, pix, bit, pix&bit != 0, aa, attr, attr&7, attr&7 == 7)
		t.Logf("   COORDS=$%02X,$%02X PFLAG=%02X ATTRT=%02X MASKT=%02X writes(%d): %v",
			emu.mem.Read(0x5C7D), emu.mem.Read(0x5C7E), emu.mem.Read(0x5C91),
			emu.mem.Read(0x5C8F), emu.mem.Read(0x5C90), len(wlog), wlog[:min(len(wlog), 12)])
		// resume
		nextStop++
		emu.paused.Store(false)
	}
	// final halt reached within a frame of last resume
	runOneFrameHeadless(emu, roms.ModelPlus2)
	emu.cpu.BreakpointCheck = nil
	t.Logf("done PC=$%04X — survivors must be read from the three PLOT lines above.", emu.cpu.PC)
}
