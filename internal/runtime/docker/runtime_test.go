package docker

import "testing"

func TestServerSpecValidate(t *testing.T) {
	valid := ServerSpec{ID: "test-1", Image: "itzg/minecraft-server:latest", MemoryMB: 2048, CPUPercent: 50, Port: 25565, ContainerPort: 25565, Env: map[string]string{"EULA": "TRUE"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		edit func(*ServerSpec)
	}{
		{"invalid ID", func(s *ServerSpec) { s.ID = "../oops" }},
		{"empty image", func(s *ServerSpec) { s.Image = "" }},
		{"image whitespace", func(s *ServerSpec) { s.Image = "busy box" }},
		{"too little memory", func(s *ServerSpec) { s.MemoryMB = 64 }},
		{"excess CPU", func(s *ServerSpec) { s.CPUPercent = 101 }},
		{"missing port", func(s *ServerSpec) { s.Port = 0 }},
		{"invalid env", func(s *ServerSpec) { s.Env = map[string]string{"BAD-KEY": "x"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.edit(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
func TestContainerNames(t *testing.T) {
	if got := containerName("abc"); got != "tyravex-abc" {
		t.Fatal(got)
	}
	if got := volumeName("abc"); got != "tyravex-data-abc" {
		t.Fatal(got)
	}
}
