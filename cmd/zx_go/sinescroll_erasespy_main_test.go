package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollEraseSpyMain — v4.1 ghost-trail hunt: does eraseband actually
// run every frame, over the right rows? Hooks eraseband/walk/bandsave.
func TestSinescrollEraseSpyMain(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
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
	var base uint16
	var code []byte
	for _, b := range blocks {
		if d, ok := b[1].([]byte); ok && len(d) > 13000 {
			base = b[0].(uint16)
			code = d
		}
	}
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
	emu.cpu.PC = base

	erases, walks, saves := 0, 0, 0
	bandRow90 := ((90&7)<<8) + ((90&0x38)<<2) + ((90&0xC0)<<5)
	var preErase byte
	emu.cpu.AddPreFetchHook("erspy", func(pc uint16) {
		switch pc {
		case syms["eraseband"]:
			erases++
			// snapshot row90 cols 8..16 of BOTH banks before erase
			var s [2]byte
			s[0] = emu.mem.RAM8KPage(10)[bandRow90+8]
			s[1] = emu.mem.RAM8KPage(14)[bandRow90+8]
			preErase = s[0]
			if erases <= 8 {
				fmt.Printf("ERASE#%d otmin=%d otmax=%d b5col8=%02X b7col8=%02X\n", erases,
					emu.mem.Read(syms["otmin"]), emu.mem.Read(syms["otmax"]), s[0], s[1])
			}
		case syms["walk"]:
			walks++
		case syms["bandsave"]:
			saves++
			if saves <= 8 {
				p := uint16(emu.mem.Read(syms["p"]))
				n5 := 0
				for x := 8; x < 24; x++ {
					n5 += int(emu.mem.RAM8KPage(10)[bandRow90+x])
				}
				fmt.Printf("SAVE#%d p=%d walks=%d erases=%d b5row90sum=%d preWas=%02X\n", saves, p, walks, erases, n5, preErase)
			}
		}
	})
	loop := syms["loop"]
	frames := 0
	for frames < 40 {
		steps := 0
		for {
			steps++
			if steps > 300000 {
				t.Fatal("runaway")
			}
			emu.cpu.StepInstructionWithIRQ()
			if emu.cpu.PC == loop && steps > 20 {
				break
			}
		}
		frames++
	}
	fmt.Printf("DONE erases=%d walks=%d saves=%d frames=%d\n", erases, walks, saves, frames)
}
