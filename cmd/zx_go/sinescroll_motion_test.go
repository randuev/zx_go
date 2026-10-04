package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollMotionAxis — Seva on real +2: "letters scroll top to down
// rather than right to left". Measure the actual ink-motion axis in the
// emulator: per-frame centroid (x̄,ȳ), band row range, and a cross-frame
// correlation that says which axis the content moves on. If the emulator
// says clean horizontal and hardware says vertical, the fault is hardware
// vsync/paint-overrun exposure, not the geometry tables.
func TestSinescrollMotionAxis(t *testing.T) {
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
	raw, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
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

	type frameStat struct {
		f                int
		p                int
		lvl             byte
		xbar, ybar     float64
		ys0, ys1       int
		rows             [192]int
		cols             [256]int
	}
	var stats []frameStat
	for f := 0; f < 160; f++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		page := emu.mem.RAM8KPage(10)
		if emu.mem.Read(syms["bbk"])&0x08 != 0 {
			page = emu.mem.RAM8KPage(14)
		}
		var st frameStat
		st.f = f
		st.p = int(emu.mem.Read(syms["p"])) | int(emu.mem.Read(syms["p"]+1))<<8
		st.lvl = emu.mem.Read(syms["lvl"])
		st.ys0, st.ys1 = 192, -1
		n := 0
		var sx, sy float64
		for y := 0; y < 192; y++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
			for xb := 0; xb < 32; xb++ {
				v := page[o+xb]
				if v == 0 {
					continue
				}
				for b := 0; b < 8; b++ {
					if v&(0x80>>b) != 0 {
						x := xb*8 + b
						n++
						sx += float64(x)
						sy += float64(y)
						st.rows[y]++
						st.cols[x]++
					}
				}
			}
		}
		if n == 0 {
			continue
		}
		st.xbar, st.ybar = sx/float64(n), sy/float64(n)
		for y := 0; y < 192; y++ {
			if st.rows[y] > 0 {
				if y < st.ys0 {
					st.ys0 = y
				}
				if y > st.ys1 {
					st.ys1 = y
				}
			}
		}
		stats = append(stats, st)
	}
	fmt.Printf("frames=%d\n", len(stats))
	if len(stats) < 10 {
		t.Fatalf("not enough frames with ink")
	}
	// motion: consecutive-frame centroid deltas
	var dxsum, dysum float64
	dxn, dyn := 0, 0
	for i := 1; i < len(stats); i++ {
		if stats[i].lvl == stats[i-1].lvl {
			dxsum += stats[i].xbar - stats[i-1].xbar
			dysum += stats[i].ybar - stats[i-1].ybar
			dxsum++
			dxn++
			dyn++
		}
	}
	fmt.Printf("centroid dx/frame=%.2f dy/frame=%.2f (over %d same-lvl pairs)\n",
		dxsum/float64(max(1, dxn)), dysum/float64(max(1, dyn)), dxn)
	// band range stability + per-column height walk (standing vs crawling band)
	for i := 0; i < len(stats); i++ {
		if i%16 == 0 || i == len(stats)-1 {
			fmt.Printf("f%-3d p=%-4d lvl%d xbar=%6.1f ybar=%6.1f bandY=%d..%d\n",
				stats[i].f, stats[i].p, stats[i].lvl, stats[i].xbar, stats[i].ybar, stats[i].ys0, stats[i].ys1)
		}
	}
	// left-edge of ink (leftmost lit pixel column): should advance LEFT as text flows
	fmt.Print("leftEdge: ")
	for i := 0; i < len(stats); i += 8 {
		le := -1
		for x := 0; x < 256; x++ {
			if stats[i].cols[x] > 0 {
				le = x
				break
			}
		}
		fmt.Printf("f%d:%d ", stats[i].f, le)
	}
	fmt.Println()
	// topmost band row trajectory — the vertical-scroll signature would make
	// the whole band's y-range walk downward monotonically.
	fmt.Print("bandTop: ")
	for i := 0; i < len(stats); i += 8 {
		fmt.Printf("f%d:%d ", stats[i].f, stats[i].ys0)
	}
	fmt.Println()
}
