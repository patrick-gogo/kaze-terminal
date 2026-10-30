// Command chime generates assets/sounds/chime.wav: a clear two-tone chime, loud
// enough to hear over music or a call.
// Run from the repo root: go run ./tools/chime
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const (
	rate     = 44100
	duration = 0.7
	fadeIn   = 0.005 // avoids a click at the start
	peakDB   = -1.0
)

func main() {
	out := filepath.Join("assets", "sounds", "chime.wav")
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, wav(samples()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// samples mixes a 880 Hz tone with a 1320 Hz tone that enters 0.1s later,
// both decaying exponentially, scaled so the loudest sample sits at peakDB.
func samples() []int16 {
	n := int(rate * duration)
	f := make([]float64, n)
	var peak float64
	for i := range f {
		t := float64(i) / rate
		v := math.Sin(2*math.Pi*880*t) * math.Exp(-t*4)
		if t >= 0.1 {
			u := t - 0.1
			v += 0.7 * math.Sin(2*math.Pi*1320*u) * math.Exp(-u*5)
		}
		if t < fadeIn {
			v *= t / fadeIn
		}
		// Short fade-out so the file ends at silence.
		if rest := duration - t; rest < 0.02 {
			v *= rest / 0.02
		}
		f[i] = v
		peak = math.Max(peak, math.Abs(v))
	}
	scale := math.Pow(10, peakDB/20) * 32767 / peak
	s := make([]int16, n)
	for i, v := range f {
		s[i] = int16(math.Round(v * scale))
	}
	return s
}

func wav(s []int16) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	dataLen := uint32(len(s) * 2)
	b.WriteString("RIFF")
	w(36 + dataLen)
	b.WriteString("WAVEfmt ")
	w(uint32(16))       // fmt chunk size
	w(uint16(1))        // PCM
	w(uint16(1))        // mono
	w(uint32(rate))     // sample rate
	w(uint32(rate * 2)) // bytes per second
	w(uint16(2))        // block align
	w(uint16(16))       // bits per sample
	b.WriteString("data")
	w(dataLen)
	w(s)
	return b.Bytes()
}
