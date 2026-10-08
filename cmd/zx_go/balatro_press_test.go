package main

import (
	"testing"
)

// pressOnce performs one complete engine-visible key stroke: press, run until
// scanKey consumes it (REL arms), release, run until the release is seen
// (REL clears). This mirrors how a human keydown survives zx_go's
// auto-releasing injection and guarantees every stroke produces exactly one
// EV_* to mainLoop.
func pressOnce(t *testing.T, emu *emulator, syms map[string]uint16, name string) {
	t.Helper()
	k, ok := balatroKeys[name]
	if !ok {
		t.Fatalf("unknown key %s", name)
	}
	rel := syms["REL"]
	// REL arms ONLY inside scanKey/skEmit, so REL==1 proves the scan ran.
	// (PC==scanKey re-entry cannot be required: quit events leave the demo
	// loop in the very pass that arms REL.)
	emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
	armed := false
	for i := 0; i < 4000000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if balPeek(emu, rel) == 1 {
			armed = true
			break
		}
	}
	if !armed {
		t.Fatalf("pressOnce(%s): REL never armed", name)
	}
	emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
	// release observed = REL clears, OR the engine completed a quit:
	// halt pad $BF00 (bootBalatro sentinel) or a ROM address.
	for i := 0; i < 4000000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		if balPeek(emu, rel) == 0 {
			return
		}
		if emu.cpu.PC == 0xBF00 || emu.cpu.PC < 0x8000 {
			return
		}
	}
	t.Fatalf("pressOnce(%s): REL never cleared", name)
}
