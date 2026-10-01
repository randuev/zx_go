package main

// Throwaway: full main-loop cycle cost with sscroll neutered (RET stub),
// to isolate non-scroller overhead per frame. Delete after budget locked.

import (
	"fmt"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestSnowLoopCycle(t *testing.T) {
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
		if len(p) >= 16 && p[0] == 0xFF && n > 5000 {
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
	// neuter sscroll + kfull
	sscroll := uint16(0x83D0)
	if os.Getenv("SNOW_GEN") == "v4b" {
		sscroll = 0x83D0 // same address in this generation
	}
	emu.mem.Write(sscroll, 0xC9)
	emu.mem.Write(0x8375, 0xAF)
	emu.mem.Write(0x8376, 0xC9)

	var cyc []int64
	var tPrev int64 = -1
	var busy []int64
	var tINT int64 = -1
	var haltT int64 = -1
	var kfullHits int
	emu.cpu.AddPreFetchHook("cyc", func(pc uint16) {
		switch pc {
		case 0x8989: // ISR entry
			tINT = int64(emu.cpu.Tstates())
		case 0x8118: // HALT in loop — end of busy window
			haltT = int64(emu.cpu.Tstates())
			if tINT >= 0 && haltT >= tINT {
				busy = append(busy, haltT-tINT)
				tINT = -1
			}
		case 0x8114:
			t := int64(emu.cpu.Tstates())
			if tPrev >= 0 && t >= tPrev {
				cyc = append(cyc, t-tPrev)
			}
			tPrev = t
		case 0x8375:
			kfullHits++
		}
	})
	for f := 0; f < 400; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	var mx, sum, mn int64 = 0, 0, 1 << 62
	for _, v := range cyc {
		sum += v
		if v > mx {
			mx = v
		}
		if v < mn {
			mn = v
		}
	}
	if len(cyc) == 0 {
		t.Fatal("no cycles")
	}
	fmt.Printf("NONSCROLL loop: n=%d avg=%d min=%d max=%d kfull=%d\n",
		len(cyc), sum/int64(len(cyc)), mn, mx, kfullHits)
	var bmx, bsum int64
	for _, v := range busy {
		bsum += v
		if v > bmx {
			bmx = v
		}
	}
	if len(busy) > 0 {
		fmt.Printf("BUSY/frame(no scroller): n=%d avg=%d max=%d\n",
			len(busy), bsum/int64(len(busy)), bmx)
	}
}
