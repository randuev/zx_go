package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"os"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollProofShot — renders the shipped kiosk tap's painted bank-5
// to a 4x-nearest-neighbour PNG for delivery proof (Seva: "send me the
// photo ... so I don't have to load anything").
func TestSinescrollProofShot(t *testing.T) {
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
	loop := uint16(0x80FC)
	for f := 0; f < 60; f++ {
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
	}
	Z := 4
	Wd, Hd := 256*Z, 192*Z
	// scanline raw, filter 0
	img := make([]byte, 0, Hd*(1+Wd*3))
	for y := 0; y < 192; y++ {
		o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5)
		for zy := 0; zy < Z; zy++ {
			img = append(img, 0)
			for xb := 0; xb < 32; xb++ {
				v := emu.mem.RAM8KPage(10)[o+xb]
				for bt := 0; bt < 8; bt++ {
					pix := []byte{0, 0, 0}
					if v&(0x80>>bt) != 0 {
						pix = []byte{0x66, 0xff, 0x66}
					}
					for zz := 0; zz < Z; zz++ {
						img = append(img, pix...)
					}
				}
			}
		}
	}
	var body bytes.Buffer
	zw2 := zlib.NewWriter(&body)
	zw2.Write(img)
	zw2.Close()
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
		return b.Bytes()
	}
	var png bytes.Buffer
	png.Write([]byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10})
	ihdr := new(bytes.Buffer)
	binary.Write(ihdr, binary.BigEndian, uint32(Wd))
	binary.Write(ihdr, binary.BigEndian, uint32(Hd))
	ihdr.Write([]byte{8, 2, 0, 0, 0})
	png.Write(chunk("IHDR", ihdr.Bytes()))
	png.Write(chunk("IDAT", body.Bytes()))
	png.Write(chunk("IEND", nil))
	if err := os.WriteFile("/tmp/sinescroll_proof_v41.png", png.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("wrote /tmp/sinescroll_proof_v41.png")
}
