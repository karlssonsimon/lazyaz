package ui

import "testing"

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name        string
		blobName    string
		contentType string
		want        string
	}{
		{name: "json extension", blobName: "data.json", want: "json"},
		{name: "xml content type", blobName: "payload.bin", contentType: "application/xml", want: "xml"},
		{name: "csv extension", blobName: "report.csv", want: "csv"},
		{name: "unknown", blobName: "notes.txt", want: "text"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectLanguage(tc.blobName, tc.contentType)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIsProbablyBinary(t *testing.T) {
	if IsProbablyBinary([]byte("hello\nworld")) {
		t.Fatal("expected plain text to be non-binary")
	}

	if !IsProbablyBinary([]byte{0x00, 0x01, 0x02}) {
		t.Fatal("expected null/control bytes to be binary")
	}
}
