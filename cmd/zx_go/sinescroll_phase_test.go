package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollPhaseProfile — real hardware marches vertically; every
// repaint that crosses a frame boundary is broken physics. Profile:
// per-frame erase-phase T, glyph-phase T, total; histogram by lvl; count
// frames > 70908 (real +2). This sizes the cut needed.
func TestSinescrollPhaseProfile(t *testing.T) {
	syms := map[string]uint16{}
	symsMap := map[uint16]string{}
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
	symsMap[uint16(v)] = name
			}
		}
	}
	raw, _ := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.tap")
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

	loopTop := syms["loop"]
	haltPC := syms["lhalt"]
	band := syms["eraseband"]
	bsave := syms["bandsave"]
	gloop := syms["gloop"]

	var tot, era, gly []int
	hist := map[uint16]int{}
	totalSteps := map[uint16]int{}
	var lvls []byte
	for frame := 0; frame < 300; frame++ {
		// one loop iteration = park..park: step until back at loopTop
		t0 := emu.cpu.Tstates()
		tBand, tBsave, tGloop0, tGloop1, tEraseEnd := 0, 0, 0, 0, 0
			nBand, nWdone, nGloop := 0, 0, 0
		n := 0
		for {
			tBefore := int(emu.cpu.Tstates())
			emu.cpu.StepInstructionWithIRQ()
			dt := int(emu.cpu.Tstates()) - tBefore
			pc := emu.cpu.PC
			if pc == band {
				tBand = int(emu.cpu.Tstates())
				nBand++
			}
			if pc == gloop {
				nGloop++
			}
			if pc >= syms["loop"] && pc <= 0xB4C0 {
				hist[pc] += dt
				totalSteps[pc]++
			}

			if pc == gloop && tGloop0 == 0 {
				tGloop0 = int(emu.cpu.Tstates())
			}
			if pc == syms["w_done"] {
				nWdone++
			}
			tGloop1 = int(emu.cpu.Tstates())
			if pc == bsave {
				tBsave = int(emu.cpu.Tstates())
			}
			if pc == gloop && tEraseEnd == 0 {
				tEraseEnd = int(emu.cpu.Tstates())
			}
			n++
			if pc == loopTop && n > 20 {
				break
			}
			if n > 300000 {
				t.Fatalf("frame runaway")
			}
		}
		_ = haltPC
		tt := int(emu.cpu.Tstates() - t0)
		tot = append(tot, tt)
		lvls = append(lvls, emu.mem.Read(syms["lvl"]))
		e := 0
		if tEraseEnd > 0 && tBand > 0 {
			e = tEraseEnd - tBand
		} else if tBsave > 0 && tBand > 0 {
			e = tBsave - tBand
		}
		era = append(era, e)
		g := 0
		if tGloop1 > tGloop0 {
			g = tGloop1 - tGloop0
		}
		gly = append(gly, g)
		if frame < 8 || frame%40 == 0 {
			fmt.Printf("f%d lvl%d tot=%d band=%d wdone=%d ngloop=%d erase=%d gloop=%d\n",
				frame, lvls[len(lvls)-1], tt, nBand, nWdone, nGloop, e, g)
		}
	}
	var h0, h1 []int
	for i := range tot {
		if lvls[i] == 0 {
			h0 = append(h0, tot[i])
		} else {
			h1 = append(h1, tot[i])
		}
	}
	sort.Ints(h0)
	sort.Ints(h1)
	regions := [][3]string{
		{"loop..lhalt", "loop", "lhalt"},
		{"lhalt..gloop", "lhalt", "gloop"},
		{"gloop..w8row", "gloop", "w8row"},
		{"w8row..gbump", "w8row", "gbump"},
		{"gbump..w_done", "gbump", "w_done"},
		{"w_done..eraseband", "w_done", "eraseband"},
		{"eraseband..bandsave", "eraseband", "bandsave"},
		{"bandsave..end", "bandsave", "end"},
	}
	fmt.Println("REGION cycles/frame avg over", len(tot), "frames:")
	for _, r := range regions {
		lo := int(syms[r[1]])
		hi := int(syms[r[2]])
		if r[2] == "end" {
			hi = 0xB4C0
		}
		t, n := 0, 0
		for a, v := range hist {
			if int(a) >= lo && int(a) <= hi {
				t += v
				n += totalSteps[a]
			}
		}
		fmt.Printf("  %-20s %7d/frame  exec=%d\n", r[0], t/len(tot), n)
	}
	sum := 0
	for _, v := range hist {
		sum += v
	}
	fmt.Printf("hist-sum=%d/frame\n", sum/len(tot))
	fmt.Printf("frames=%d\n", len(tot))
	pctl := func(name string, v []int) {
		if len(v) == 0 {
			fmt.Printf("%s: none\n", name)
			return
		}
		fmt.Printf("%s n=%d med=%d p90=%d max=%d\n",
			name, len(v), v[len(v)/2], v[len(v)*9/10], v[len(v)-1])
	}
	pctl("h8 total", h0)
	pctl("h16 total", h1)
	pctl("erase(med-max)", era)
	fmt.Printf("erase med=%d max=%d gloop max=%d\n", era[len(era)/2], maxOf(era), maxOf(gly))
	over70 := 0
	over66 := 0
	for _, v := range tot {
		if v > 70908 {
			over70++
		}
		if v > 66000 {
			over66++
		}
	}
	fmt.Printf("frames>70908=%d/%d  >66000=%d\n", over70, len(tot), over66)
	for i := 0; i < len(tot); i++ {
		if tot[i] > 70908 {
			fmt.Printf("OVER f%d lvl%d tot=%d erase=%d gloop=%d\n", i, lvls[i], tot[i], era[i], gly[i])
			break
		}
	}
}

func maxOf(v []int) int {
	m := 0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}
