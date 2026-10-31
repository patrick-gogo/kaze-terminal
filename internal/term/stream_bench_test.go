package term

import (
	"bytes"
	"fmt"
	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
	"io"
	"testing"
)

func BenchmarkLogStream(b *testing.B) {
	for _, sz := range [][2]int{{120, 40}, {240, 80}} {
		b.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(b *testing.B) {
			row := []byte("the quick brown fox jumps over the lazy dog\r\n")
			data := bytes.Repeat(row, (64<<10)/len(row))
			p := vt.NewParser(NewApplier(screen.NewWithScrollback(sz[0], sz[1], 1000), io.Discard))
			p.Feed(data)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.Feed(data)
			}
		})
	}
}
