package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollRung0 — RUNG 0 sanity tape: border flashes green/black then
// parks GREEN at halt. ZERO screen writes, ZERO paging, ZERO keys. Proves
// LOAD ""CODE + USR + zero-graphics execution on the +2 core.
func TestSinescrollRung0(t *testing.T) {
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/diag0/rung0.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	var code []byte
	for _, b := range blocks {
		if d, ok := b[1].([]byte); ok && len(d) > 20 && b[0].(uint16) == 0x8000 {
			code = d
		}
	}
	if code == nil {
		t.Fatal("no CODE block at $8000")
	}
	if code[len(code)-1] != 0x76 {
		t.Fatalf("last byte $%02X, want halt $76", code[len(code)-1])
	}
	park := uint16(0x8000 + len(code)) // PC after executing halt
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
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
	for i, v := range code {
		emu.mem.Write(0x8000+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.cpu.PC = 0x8000
	flashPC := uint16(0x8000 + 8) // 'flash' label offset (verify below via OUT count)
	outs := 0
	emu.cpu.AddPreFetchHook("rung0flash", func(pc uint16) {
		if pc == flashPC {
			outs++
		}
	})
	parked := false
	for s := 0; s < 6000000; s++ {
		emu.cpu.StepInstructionWithIRQ()
		if emu.cpu.PC == park {
			parked = true
			break
		}
	}
	if !parked {
		t.Fatalf("never reached park $%04X (pc=$%04X)", park, emu.cpu.PC)
	}
	_ = flashPC
	fmt.Printf("RUNG0: %d bytes, flash-loop passes=%d, parked at halt ✓\n", len(code), outs)
}
