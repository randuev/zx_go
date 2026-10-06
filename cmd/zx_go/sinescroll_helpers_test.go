package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// loadSinescrollSyms parses sjasmplus EQU output into a name->addr map.
func loadSinescrollSyms(t *testing.T) map[string]uint16 {
	t.Helper()
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
	if err != nil {
		t.Fatalf("read sym: %v", err)
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
	for _, n := range []string{"loop", "lhalt", "pclean", "eraseband", "walk", "w_done", "bandsave", "isr", "lvl", "p", "pp", "flipdirty"} {
		if _, ok := syms[n]; !ok {
			t.Fatalf("symbol %s missing", n)
		}
	}
	return syms
}

// bootSinescroll boots the +2 core, loads the code block directly to RAM
// (bypassing the flaky headless tape trap — established harness recipe),
// and starts the demo.
func bootSinescroll(t *testing.T, syms map[string]uint16, tapPath string) *emulator {
	t.Helper()
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
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
	if code == nil {
		t.Fatal("no code block")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
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
	return emu
}
