package app

import "testing"

func TestParentPrefix(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{name: "root", input: "", output: ""},
		{name: "single folder", input: "foo/", output: ""},
		{name: "nested", input: "foo/bar/", output: "foo/"},
		{name: "nested without trailing slash", input: "foo/bar", output: "foo/"},
		{name: "deep", input: "foo/bar/baz/", output: "foo/bar/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentPrefix(tc.input); got != tc.output {
				t.Fatalf("expected %q, got %q", tc.output, got)
			}
		})
	}
}

func TestTrimPrefixForDisplay(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		prefix string
		want   string
	}{
		{name: "no prefix", value: "folder/file.txt", prefix: "", want: "folder/file.txt"},
		{name: "with prefix", value: "folder/file.txt", prefix: "folder/", want: "file.txt"},
		{name: "same as prefix", value: "folder/", prefix: "folder/", want: "folder/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimPrefixForDisplay(tc.value, tc.prefix); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestBlobSearchPrefix(t *testing.T) {
	tests := []struct {
		name          string
		currentPrefix string
		query         string
		want          string
	}{
		{name: "plain query at root", currentPrefix: "", query: "foo", want: "foo"},
		{name: "query scoped to current prefix", currentPrefix: "logs/", query: "2026", want: "logs/2026"},
		{name: "query already includes prefix", currentPrefix: "logs/", query: "logs/2026", want: "logs/2026"},
		{name: "leading slash means absolute", currentPrefix: "logs/", query: "/archive/", want: "archive/"},
		{name: "windows slash normalized", currentPrefix: "logs/", query: "2026\\02", want: "logs/2026/02"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blobSearchPrefix(tc.currentPrefix, tc.query); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
