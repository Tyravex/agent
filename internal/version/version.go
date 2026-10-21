package version

import (
	"encoding/json"
	"runtime"
	"strings"
)

var (
	Number    = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func Current() Info {
	return Info{
		Version:   strings.TrimPrefix(Number, "v"),
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func JSON() ([]byte, error) {
	return json.MarshalIndent(Current(), "", "  ")
}
