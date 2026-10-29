package term

import (
	"io"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
)

func TestPromptSeqCountsOnlyPromptStartMarks(t *testing.T) {
	a := NewApplier(screen.New(10, 3), io.Discard)
	p := vt.NewParser(a)
	if a.PromptSeq() != 0 {
		t.Fatalf("seq %d before any mark", a.PromptSeq())
	}
	p.Feed([]byte("\x1b]133;A\x07"))
	if a.PromptSeq() != 1 {
		t.Fatalf("seq %d after one 133;A", a.PromptSeq())
	}
	p.Feed([]byte("\x1b]133;B\x07\x1b]133;C\x07\x1b]133;D;0\x07\x1b]133;AB\x07\x1b]133\x07"))
	if a.PromptSeq() != 1 {
		t.Fatalf("other 133 marks moved seq to %d", a.PromptSeq())
	}
	p.Feed([]byte("\x1b]133;A;cl=m\x1b\\"))
	if a.PromptSeq() != 2 {
		t.Fatalf("seq %d after 133;A with params", a.PromptSeq())
	}
}
