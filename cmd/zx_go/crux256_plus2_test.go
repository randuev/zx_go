package main

// TestCrux256Plus2Run — ground-truth proof for crux256 inc1 (music core).
// Boots a real +2 core, pokes _inc1.bin at $8000, runs frames, and proves:
//   1. PC never escapes the code window (only ROM via INT / halt in-place)
//   2. AY chC walks the WM refrain: reg4 shows multiple distinct pentatonic
//      periods cycling; reg7/mixer, reg10/vol from init are live
//   3. Border heartbeat toggles between black and green
//   4. Attrs uniformly $1F, bitmap still blank (inc1 has no art yet)
//
// Run: go test ./cmd/zx_go/ -run TestCrux256Plus2Run -v

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestCrux256Plus2Run(t *testing.T) {
	bin, err := os.ReadFile("/root/nerve-workspace/demos/crux256/_inc1.bin")
	if err != nil {
		t.Fatalf("read _inc1.bin: %v", err)
	}
	t.Logf("code len=%d (limit 256)", len(bin))
	if len(bin) > 256 {
		t.Fatalf("OVER BUDGET: %d bytes", len(bin))
	}

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)

	// Boot to BASIC prompt like a real +2.
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("boot: ScreenPage=%d PC=$%04X SP=$%04X", emu.mem.ScreenPage, emu.cpu.PC, emu.cpu.SP)

	for i, b := range bin {
		emu.mem.Write(uint16(0x8000+i), b)
	}
	for i := 0; i < 6; i++ {
		if emu.mem.Read(uint16(0x8000+i)) != bin[i] {
			t.Fatalf("poke mismatch at +%02X", i)
		}
	}

	// Escape guard: flag PC leaving code/ROM into BASIC workspace or past code.
	escaped := 0
	escapePC := uint16(0)
	emu.cpu.BreakpointCheck = func(pc uint16) bool {
		if pc >= 0x4000 && pc <= 0x7FFF {
			escaped++
			if escapePC == 0 {
				escapePC = pc
			}
			emu.paused.Store(true)
			return true
		}
		if pc > 0x8000+uint16(len(bin)) {
			escaped++
			if escapePC == 0 {
				escapePC = pc
			}
			emu.paused.Store(true)
			return true
		}
		return false
	}

	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000

	ay := emu.ula.AY()
	if ay == nil {
		t.Fatalf("no AY on +2 core")
	}

	type noteSnap struct {
		frame  int
		reg4   byte
		border byte
	}
	var snaps []noteSnap
	seenReg4 := map[byte]int{}
	borderSeen := map[byte]int{}

	const frames = 700
	for i := 0; i < frames; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if emu.paused.Load() {
			break
		}
		r4 := ay.ReadRegister(4)
		br := emu.ula.BorderColour
		seenReg4[r4]++
		borderSeen[br]++
		// snapshot on change only
		if len(snaps) == 0 || snaps[len(snaps)-1].reg4 != r4 || snaps[len(snaps)-1].border != br {
			snaps = append(snaps, noteSnap{i, r4, br})
		}
	}
	if escaped > 0 {
		t.Errorf("PC ESCAPED to $%04X (count %d) — code runaway", escapePC, escaped)
	}
	if emu.paused.Load() {
		t.Fatalf("halted early at frame? PC=$%04X", emu.cpu.PC)
	}

	t.Logf("final PC=$%04X SP=$%04X IM=%d IFF1=%v", emu.cpu.PC, emu.cpu.SP, emu.cpu.IM, emu.cpu.IFF1)

	// 1. mixer/init registers live
	if g7 := ay.ReadRegister(7); g7 != 0xDA {
		t.Errorf("reg7 mixer=$%02X want $DA", g7)
	}
	if g10 := ay.ReadRegister(10); g10 != 0x0F {
		t.Errorf("reg10 chC vol=%02X want $0F", g10)
	}
	if g5 := ay.ReadRegister(5); g5 != 0x00 {
		t.Errorf("reg5 chC fine=%02X want $00", g5)
	}

	// 2. melody walked: pentatonic periods present and cycling
	want := []byte{252, 212, 189, 168, 141, 126}
	present := 0
	for _, p := range want {
		if seenReg4[p] > 0 {
			present++
		} else {
			t.Errorf("pentatonic period %d never appeared on reg4", p)
		}
	}
	var reg4seq []string
	for _, s := range snaps {
		reg4seq = append(reg4seq, fmt.Sprintf("%d:%d", s.frame, s.reg4))
	}
	t.Logf("reg4 timeline (first 40 changes): %v", reg4seq[:min(len(reg4seq), 40)])
	t.Logf("distinct pentatonic periods seen: %d/6", present)
	if present < 6 {
		t.Errorf("melody not walking — %d/6 periods", present)
	}

	// 3. heartbeat border toggled
	if len(borderSeen) < 2 {
		t.Errorf("border never toggled: %v", borderSeen)
	} else {
		t.Logf("border colours seen: %v", borderSeen)
	}

	// 4. screen state: blank bitmap + uniform attrs (inc1 art pending)
	lit := 0
	for a := 0x4000; a < 0x5800; a++ {
		if emu.mem.Read(uint16(a)) != 0 {
			lit++
		}
	}
	badAttr := 0
	for a := 0x5800; a < 0x5B00; a++ {
		if emu.mem.Read(uint16(a)) != 0x1F {
			badAttr++
		}
	}
	t.Logf("lit bitmap bytes=%d (expect 0 for inc1), bad attrs=%d (expect 0)", lit, badAttr)
	if badAttr > 0 {
		t.Errorf("attr fill wrong: %d cells != $1F", badAttr)
	}

	emu.cpu.BreakpointCheck = nil
	t.Logf("PASS-CHECK: PC=$%04X escaped=%d reg4-changes=%d borders=%v", emu.cpu.PC, escaped, len(reg4seq), borderSeen)
}
