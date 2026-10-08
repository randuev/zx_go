package main

import (
	"fmt"
	"testing"
)

func TestBalatroShuffleOnce(t *testing.T) {
	syms := loadBalatroSyms(t)
	emu := bootBalatro(t, syms)
	emu.mem.Write(syms["SEED"], 0xE1)
	emu.mem.Write(syms["SEED"]+1, 0xAC)
	ret := uint16(0x5CC0)
	emu.cpu.SP -= 2
	emu.mem.Write(emu.cpu.SP, byte(ret&0xFF))
	emu.mem.Write(emu.cpu.SP+1, byte(ret>>8))
	emu.cpu.PC = syms["shuffle"]
	events := 0
	seen := 0
	emu.cpu.AddPreFetchHook("sfonce", func(pc uint16) {
		if pc == syms["sfGot"] {
			seen++
			if seen <= 5 {
				fmt.Printf("SF_GOT%d j=%d k=%d\n", seen, emu.cpu.A, emu.cpu.B)
			}
		}
	})
	for i := 0; i < 3000000; i++ {
		emu.cpu.StepInstructionWithIRQ()
		for x := 0; x < 52; x++ {
			v := emu.mem.Read(syms["DECK"] + uint16(x))
			_ = v
		}
		if emu.cpu.PC == ret {
			break
		}
	}
	_ = events
	var dk [52]byte
	cnt := map[byte]int{}
	for i := 0; i < 52; i++ {
		dk[i] = emu.mem.Read(syms["DECK"] + uint16(i))
		cnt[dk[i]]++
	}
	fmt.Printf("sfGot_hits=%d deck=%v\n", seen, dk)
	for c := 0; c < 52; c++ {
		if cnt[byte(c)] != 1 {
			t.Fatalf("card %d count %d", c, cnt[byte(c)])
		}
	}
}
