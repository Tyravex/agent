package docker

import "testing"

func TestValidateContainerOwnership(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:  "both labels valid",
			value: "true|example",
		},
		{
			name:    "managed label missing",
			value:   "<no value>|example",
			wantErr: true,
		},
		{
			name:    "managed label false",
			value:   "false|example",
			wantErr: true,
		},
		{
			name:    "service label missing",
			value:   "true|<no value>",
			wantErr: true,
		},
		{
			name:    "wrong service",
			value:   "true|other",
			wantErr: true,
		},
		{
			name:    "both labels missing",
			value:   "<no value>|<no value>",
			wantErr: true,
		},
		{
			name:    "unexpected extra output",
			value:   "true|example|extra",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateContainerOwnership(tc.value, "example")

			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"validateContainerOwnership(%q): err=%v, wantErr=%v",
					tc.value,
					err,
					tc.wantErr,
				)
			}
		})
	}
}
