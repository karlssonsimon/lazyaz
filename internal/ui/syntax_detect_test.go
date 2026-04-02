package ui

import "testing"

func TestIsProbablyBinary(t *testing.T) {
	if IsProbablyBinary([]byte("hello\nworld")) {
		t.Fatal("expected plain text to be non-binary")
	}

	if !IsProbablyBinary([]byte{0x00, 0x01, 0x02}) {
		t.Fatal("expected null/control bytes to be binary")
	}
}
