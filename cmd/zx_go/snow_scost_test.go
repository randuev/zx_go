package main

// Throwaway: authoritative per-frame T accounting via PC fetch hooks.
// E0@8114 (loop head) -> INT vector fetch @8989 (first fetch after HALT
// wake) -> E3@81C5 (scroller entry) -> next E0. idle = t0..tINT (HALT
// block). work = frame - idle. Multiple loop passes per pump frame are
// collapsed; pairs with a missing leg are dropped. Delete when locked.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowScrollerCost(t *testing.T) {
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	tapPath := "/root/nerve-workspace/demos/snow/snow.tap"
	if p := os.Getenv("SNOW_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatal(err)
	}
	var code []byte
	off := 0
	for off+2 <= len(raw) {
		n := int(raw[off]) | int(raw[off+1])<<8
		off += 2
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0xFF && n > 5000 {
			code = p[1 : n-1]
		}
	}
	if code == nil {
		t.Fatal("no big CODE block")
	}
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(uint16(0x8000+i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1
	emu.mem.Write(0x8369, 0xAF)
	emu.mem.Write(0x836A, 0xC9)

	type mark struct{ t0, tint, te3 int64 }
	var m mark
	var c0, ci, ce int
	type rec struct {
		work, idle, scr, flake uint64
		s                      uint16
	}
	var recs []rec
	emu.cpu.AddPreFetchHook("cost", func(pc uint16) {
		switch pc {
		// Real cycle per pump: INT@8989 -> [kpoll|flakes] -> E3@81C5
		// -> sscroll -> E0@8114 (paint black) -> HALT until next INT.
		// Frame boundary = INT-to-INT. idle = E0..next-INT (m.t0..tint).
		case 0x8114:
			c0++
			t := int64(emu.cpu.Tstates())
			if m.tint >= 0 && m.te3 >= 0 && t > m.tint {
				recs = append(recs, rec{
					work:  uint64(t - m.tint),
					idle:  0, // filled at next INT: idle = tint(next) - t0
					scr:   uint64(t - m.te3),
					flake: uint64(m.te3 - m.tint),
					s:     uint16(emu.mem.Read(0x8CA0)),
				})
			}
			m = mark{t0: t}
		case 0x8989:
			ci++
			m.tint = int64(emu.cpu.Tstates())
			m.te3 = -1
		case 0x81D1:
			ce++
			if m.tint >= 0 && m.te3 < 0 {
				m.te3 = int64(emu.cpu.Tstates())
			}
		}
	})
	for f := 0; f < 400; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	fmt.Printf("COUNTS e0=%d int=%d e3=%d\n", c0, ci, ce)
	if len(recs) == 0 {
		t.Fatal("no complete frame marks")
	}
	var sumW, maxW uint64
	byPhase := map[uint16][]uint64{}
	worst := rec{}
	for _, r := range recs {
		sumW += r.work
		if r.work > maxW {
			maxW, worst = r.work, r
		}
		byPhase[r.s] = append(byPhase[r.s], r.work)
	}
	n := uint64(len(recs))
	fmt.Printf("FRAMES n=%d work avg=%d max=%d (worst s=%d scr=%d flake=%d idle=%d)\n",
		n, sumW/n, maxW, worst.s, worst.scr, worst.flake, worst.idle)
	ph := []int{}
	for k := range byPhase {
		ph = append(ph, int(k))
	}
	sort.Ints(ph)
	for _, k := range ph {
		v := byPhase[uint16(k)]
		var s, mm uint64
		for _, d := range v {
			s += d
			if d > mm {
				mm = d
			}
		}
		fmt.Printf("  s=%d n=%3d work avg=%6d max=%6d\n", k, len(v), s/uint64(len(v)), mm)
	}
}
