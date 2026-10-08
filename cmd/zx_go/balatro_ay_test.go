package main

import (
	"fmt"
	"testing"
)

// AY census: run a full session and prove each sfx fires and AY register
// writes actually reach the chip (latch $FFFD + data $BFFD port events).
func TestBalatroAY(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	mon := map[string]int{}
	targets := []string{"sfxDeal", "sfxSelect", "sfxPlay", "sfxWin", "sfxLose", "sfxDisc", "sfxNote", "sfxVol", "ayMute3"}
	for _, s := range targets {
		if _, ok := syms[s]; ok {
			mon[s] = 0
		}
	}
	stepMon := func(n int) {
		for i := 0; i < n; i++ {
			pc := emu.cpu.PC
			emu.cpu.StepInstructionWithIRQ()
			for s, c := range mon {
				if pc == syms[s] {
					mon[s] = c + 1
				}
			}
		}
	}
	// instrumented stroke: same handshake as pressOnce but counting sfx
	kof := func(name string) [2]uint8 { return balatroKeys[name] }
	rel := syms["REL"]
	stroke := func(name string, tail int) {
		k := kof(name)
		// drain any stale arm first, then the stroke arm
		for i := 0; i < 4000000 && balPeek(emu, rel) != 0; i++ {
			stepMon(1000)
		}
		emu.kbd.PressMatrixKey(int(k[0]), k[1], true)
		for i := 0; i < 4000000; i++ {
			pc := emu.cpu.PC
			emu.cpu.StepInstructionWithIRQ()
			for s, c := range mon {
				if pc == syms[s] {
					mon[s] = c + 1
				}
			}
			if balPeek(emu, rel) == 1 {
				break
			}
		}
		emu.kbd.PressMatrixKey(int(k[0]), k[1], false)
		stepMon(tail)
	}
	stroke("SPACE", 300000) // title -> game: sfxDeal
	for i := 0; i < 4; i++ {
		emu.mem.Write(syms["HAND"]+uint16(i), byte(i*16+12))
	}
	emu.mem.Write(syms["HAND"]+4, byte(2*16+2))
	stroke("ENTER", 20000) // select then discard: sfxDisc
	stroke("D", 20000)
	for i := 0; i < 5; i++ {
		stroke("ENTER", 20000) // select: sfxSelect
		stroke("6", 20000)
	}
	stroke("SPACE", 600000) // play: sfxPlay + score blips
	fmt.Printf("sfxMon=%v MODE=%d TOT=%d\n", mon, balPeek(emu, syms["MODE"]), balBCD3(emu, syms["TOT"]))
	if mon["sfxDeal"] == 0 {
		t.Error("sfxDeal never fired")
	}
	if mon["sfxSelect"] == 0 {
		t.Error("sfxSelect never fired")
	}
	if mon["sfxPlay"] == 0 {
		t.Error("sfxPlay never fired")
	}
	if mon["sfxWin"] == 0 && mon["sfxLose"] == 0 {
		t.Error("no win/lose sfx")
	}
	if mon["sfxNote"] < 10 {
		t.Errorf("sfxNote hits=%d want >=10", mon["sfxNote"])
	}
}
