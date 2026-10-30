package pick

import (
	"errors"
	"testing"
)

func TestParseTrimsTrailingSlashAndNewline(t *testing.T) {
	got, ok, err := parse([]byte("/Users/me/proj/\n"), nil, nil)
	if got != "/Users/me/proj" || !ok || err != nil {
		t.Fatalf("parse = %q, %v, %v", got, ok, err)
	}
}

func TestParseKeepsTheRootFolder(t *testing.T) {
	if got, ok, _ := parse([]byte("/\n"), nil, nil); got != "/" || !ok {
		t.Fatalf("parse(/) = %q, %v", got, ok)
	}
	if got, ok, _ := parse([]byte("C:\\\r\n"), nil, nil); got != `C:\` || !ok {
		t.Fatalf(`parse(C:\) = %q, %v`, got, ok)
	}
}

func TestParseCancelIsNotAnError(t *testing.T) {
	got, ok, err := parse(nil, []byte("execution error: User canceled. (-128)\n"), errors.New("exit status 1"))
	if got != "" || ok || err != nil {
		t.Fatalf("cancel = %q, %v, %v", got, ok, err)
	}
	// The Windows script prints nothing when the dialog is cancelled.
	if got, ok, err := parse([]byte("\r\n"), nil, nil); got != "" || ok || err != nil {
		t.Fatalf("empty output = %q, %v, %v", got, ok, err)
	}
}

func TestParseFailureIsAnError(t *testing.T) {
	_, ok, err := parse(nil, []byte("osascript: something broke\n"), errors.New("exit status 1"))
	if ok || err == nil || err.Error() != "osascript: something broke" {
		t.Fatalf("failure = %v, %v", ok, err)
	}
}
