package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollEraseSpy — v4.1 ghost-trail hunt: does eraseband actually
// run every frame, over the right rows? Hooks eraseband/walk/bandsave.
func TestSinescrollEraseSpy(t *testing.T) {
	syms := map[string]uint16{}
	bs, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll_diag.sym")
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
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll_diag.tap")
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
	emu.cpu.AddPreFetchHook("erspy", func(pc uint16) {
		switch pc {
		case syms["eraseband"]:
			erases++
			if erases <= 8 {
				fmt.Printf("ERASE#%d otmin=%d otmax=%d lvl=%d\n", erases,
					emu.mem.Read(syms["otmin"]), emu.mem.Read(syms["otmax"]), emu.mem.Read(syms["lvl"]))
			}
		case syms["walk"]:
			walks++
		case syms["bandsave"]:
			saves++
			if saves <= 8 {
				p := uint16(emu.mem.Read(syms["p"]))
				fmt.Printf("SAVE#%d p=%d walks=%d erases=%d\n", saves, p, walks, erases)
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
