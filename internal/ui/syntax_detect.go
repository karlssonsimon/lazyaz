package ui

import (
	"bytes"

	"golang.org/x/text/encoding/unicode"
)

// binaryThresholdPercent is the share of non-text bytes above which a
// sample is treated as binary. Compressed and encoded data spreads
// evenly over all byte values, which puts roughly a tenth of it in the
// non-text range; real text stays far below.
const binaryThresholdPercent = 5

// IsProbablyBinary reports whether data looks like something other than
// text. A viewer that refuses to show a file is worse than one that
// shows a few odd characters, so the call is made on how much of the
// sample is non-text rather than on any single byte: a log with a stray
// NUL or with terminal color codes is still a log.
func IsProbablyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	nonText := 0
	for _, b := range data {
		if isNonTextByte(b) {
			nonText++
		}
	}
	return nonText*100 > len(data)*binaryThresholdPercent
}

// isNonTextByte reports control bytes that plain text has no use for.
// Tab, the line-ending and form-feed range and ESC — the lead byte of
// the color codes in terminal logs — are text.
func isNonTextByte(b byte) bool {
	if b >= 0x20 {
		return false
	}
	if b >= 0x09 && b <= 0x0D {
		return false
	}
	return b != 0x1B
}

var (
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// HasUTF16BOM reports whether data starts with a UTF-16 byte order mark,
// which is how Windows tooling — PowerShell redirection, .NET's
// Encoding.Unicode, SQL Server exports — writes text.
func HasUTF16BOM(data []byte) bool {
	return bytes.HasPrefix(data, utf16LEBOM) || bytes.HasPrefix(data, utf16BEBOM)
}

// DecodeUTF16 converts BOM-marked UTF-16 to UTF-8, dropping the mark.
// Every other byte of UTF-16 text is a NUL, so it has to be decoded
// before it can be shown or searched as text.
func DecodeUTF16(data []byte) ([]byte, error) {
	decoder := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()
	return decoder.Bytes(data)
}
