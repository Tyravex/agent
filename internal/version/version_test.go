package version

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestCurrent(t *testing.T) {
	info := Current()

	if info.Version == "" {
		t.Fatal("version cannot be empty")
	}

	if info.OS != runtime.GOOS {
		t.Fatalf("unexpected OS: %s", info.OS)
	}

	if info.Arch != runtime.GOARCH {
		t.Fatalf("unexpected architecture: %s", info.Arch)
	}
}

func TestJSON(t *testing.T) {
	data, err := JSON()
	if err != nil {
		t.Fatal(err)
	}

	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}

	if info.Version == "" || info.GoVersion == "" {
		t.Fatal("missing version information")
	}
}

func TestVersionPrefix(t *testing.T) {
	original := Number
	t.Cleanup(func() {
		Number = original
	})

	Number = "v0.2.1"

	if Current().Version != "0.2.1" {
		t.Fatalf("unexpected version: %s", Current().Version)
	}
}
