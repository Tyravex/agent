package services

import "testing"

func validSpec() Spec {
	return Spec{ID: "service_1", Image: "eclipse-temurin:21-jre", Startup: "java -jar server.jar", MemoryMB: 1024, CPUPercent: 50, DiskMB: 4096}
}

func TestSpecValid(t *testing.T) {
	s := validSpec()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	name, err := s.ContainerName()
	if err != nil || name != "tyravex-service_1" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestSpecRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Spec)
	}{
		{"path traversal", func(s *Spec) { s.ID = "../other" }},
		{"empty image", func(s *Spec) { s.Image = "" }},
		{"image whitespace", func(s *Spec) { s.Image = "busybox latest" }},
		{"empty startup", func(s *Spec) { s.Startup = " " }},
		{"memory too small", func(s *Spec) { s.MemoryMB = 1 }},
		{"CPU over 100", func(s *Spec) { s.CPUPercent = 101 }},
		{"zero disk", func(s *Spec) { s.DiskMB = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpec()
			tc.edit(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
