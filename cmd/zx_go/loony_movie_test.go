package main

// loony_movie_test.go — LOONYMOVIE=1: native tape-trap boot (+2, SAME tape as
// Seva), direct-step, capture displayed-bank PNG per N steps + census.
// Proves frame-to-frame animation (or its absence) for the v13.5 "completely
// stuck while music plays" report. Output: /tmp/loony_movie/m*.png + meta log.
import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func TestLoonyMovie(t *testing.T) {
	if os.Getenv("LOONYMOVIE") == "" {
		t.Skip("set LOONYMOVIE=1")
	}
	os.MkdirAll("/tmp/loony_movie", 0o755)
	mapRaw, _ := os.ReadFile("/tmp/loony.sym")
	syms := map[string]uint16{}
	re := regexp.MustCompile(`(\w+): (?:EQU|DEF[SE]) 0x0000([0-9A-Fa-f]{4})`)
	for _, m := range re.FindAllStringSubmatch(string(mapRaw), -1) {
		var v uint64
		fmt.Sscanf(m[2], "%x", &v)
		syms[m[1]] = uint16(v)
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	nf.headless = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatal(err)
	}
	emu.paused.Store(false)
	if err := loadTapeFile(emu, "/root/nerve-workspace/demos/loony/loony.tap"); err != nil {
		t.Fatal(err)
	}
	installTapeTrap(emu)
	landed := false
	for attempt := 0; attempt < 4 && !landed; attempt++ {
		for f := 0; f < 900 && !landed; f++ {
			if f == 40 {
				emu.kbd.PressMatrixKey(6, 0x01, true)
			}
			if f == 80 {
				emu.kbd.PressMatrixKey(6, 0x01, false)
			}
			if f == 160 {
				emu.kbd.PressMatrixKey(6, 0x01, true)
			}
			if f == 200 {
				emu.kbd.PressMatrixKey(6, 0x01, false)
			}
			runOneFrameHeadless(emu, roms.ModelPlus2)
			if emu.mem.Read(0x8000) == 0xF3 {
				landed = true
				// CRITICAL: release ENTER the instant code lands. Native
				// land is frame ~168 > the second press (160) — without this
				// the loop exits with ENTER held and the demo's key poll
				// quits to BASIC right after its grace (quit seen at t=40).
				emu.kbd.PressMatrixKey(6, 0x01, false)
			}
		}
		if !landed {
			fmt.Printf("retry boot attempt %d\n", attempt)
		}
	}
	if !landed {
		t.Fatal("no native land")
	}
	fmt.Printf("landed frame, t=%d\n", emu.mem.Read(syms["t"]))

	save := func(idx int) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, 256, 192))
		// Read the DISPLAYED bank through the logical $4000 window (MMU
		// already pages bit3-selected screen bank there). NOTE: RAM8KPage
		// takes an 8K PAGE index (bank5=page 10), and a bank-relative off
		// spans 16K across two pages — the old save() read the code bank
		// and produced all-black frames despite live DIFF signatures.
		for y := 0; y < 192; y++ {
			for c := 0; c < 32; c++ {
				off := uint16((y>>6)*0x800 + (y&7)*0x100 + ((y>>3)&7)*0x20) + uint16(c)
				b := emu.mem.Read(0x4000 + off)
				for px := 0; px < 8; px++ {
					on := b&(1<<(7-uint(px))) != 0
					col := color.RGBA{}
					if on {
						col = color.RGBA{255, 255, 255, 255}
					}
					img.Set(c*8+px, y, col)
				}
			}
		}
		f, _ := os.Create(fmt.Sprintf("/tmp/loony_movie/m%03d.png", idx))
		png.Encode(f, img)
		f.Close()
		return img
	}

	prevSig := ""
	diffCount := 0
	N := 60
	stepPer := 25000
	for k := 0; k < N; k++ {
		for s := 0; s < stepPer; s++ {
			emu.cpu.StepInstructionWithIRQ()
		}
		save(k)
		sig := ""
		// crude signature: sum of every 64th byte of displayed bank + state
		sum := 0
		// sample the DISPLAYED 16K via logical $4000 window (bank already paged)
		for i := 0; i < 0x4000; i += 64 {
			sum += int(emu.mem.Read(uint16(0x4000 + i)))
		}
		sig = fmt.Sprintf("%d t=%d bbk=%d flipcnt=%d", sum, emu.mem.Read(syms["t"]), emu.mem.Read(syms["bbk"]), emu.mem.Read(syms["flipcnt"]))
		d := ""
		if prevSig != "" {
			if sig == prevSig {
				d = "SAME"
			} else {
				d = "DIFF"
				diffCount++
			}
		}
		prevSig = sig
		fmt.Printf("cap %02d %s %s pc=$%04X\n", k, sig, d, emu.cpu.PC)
	}
	fmt.Printf("MOVIE: %d/%d transitions DIFF\n", diffCount, N-1)
}
