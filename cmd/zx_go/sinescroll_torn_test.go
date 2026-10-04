package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollTornAudit — v4.0 hardware law: the ULA displays the bank PA
// selects at $4000; the CPU paints the HIDDEN bank via $C000 (page bits
// 0-2). v2.4 kept page bits stuck at $07, so at bbk=$0F PA=1 displayed b7
// while $C000 STILL mapped b7 -> paint wrote the LIVE bank every other
// cycle; each refresh caught the paint front sweeping down = letters
// marching top-to-bottom (Seva's report). v4.0 flips $07<->$0D: PA and page
// swap together, so $C000 always maps the hidden bank. Audit: across N
// frames the pair (page bits, display bank) must never overlap, and both
// states must occur.
func TestSinescrollTornAudit(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(rest[6:], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseTap(raw)
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
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
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0)
	emu.mem.Write(0xFFFF, 0)
	emu.cpu.PC = base

	torn := 0
	states := map[byte]int{}
	gloop := syms["gloop"]
	loop := syms["loop"]
	pa := syms["pg"] // not present; use bbk
	_ = pa
	_ = gloop
	for f := 0; f < 200; f++ {
		steps := 0
		for {
			steps++
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
		}
		bbk := emu.mem.Read(syms["bbk"])
		states[bbk]++
		disp := 5
		if bbk&0x08 != 0 {
			disp = 7
		}
		if int(bbk&0x07) == disp {
			torn++
		}
	}
	fmt.Printf("AUDIT: states=%v torn-frames=%d\n", states, torn)
	if torn != 0 {
		t.Fatalf("TORN: %d frames where $C000 page == displayed bank", torn)
	}
	if states[0x07] == 0 || states[0x0D] == 0 {
		t.Fatalf("flip states wrong: %v", states)
	}
}
