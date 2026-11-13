package dockerlab

import (
	"testing"
)

func TestLimitedBufferCapsCapturedOutput(t *testing.T) {
	buffer := newLimitedBuffer(4)
	if count, err := buffer.Write([]byte("abcdef")); err != nil || count != 6 {
		t.Fatalf("Write() = %d, %v", count, err)
	}
	if got := buffer.String(); got != "abcd" || !buffer.exceeded {
		t.Fatalf("limited buffer = %q, exceeded=%v", got, buffer.exceeded)
	}
}
