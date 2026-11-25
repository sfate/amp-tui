package doctor

import "testing"

func TestCheckYTDLPVersion(t *testing.T) {
	for _, test := range []struct {
		version string
		wantErr bool
	}{{"2025.11.11", true}, {"2025.11.12", false}, {"2026.08.19", false}, {"not-a-version", true}} {
		t.Run(test.version, func(t *testing.T) {
			if err := checkYTDLPVersion(test.version); (err != nil) != test.wantErr {
				t.Fatalf("checkYTDLPVersion(%q) error = %v, wantErr %t", test.version, err, test.wantErr)
			}
		})
	}
}

func TestCheckDenoVersion(t *testing.T) {
	for _, test := range []struct {
		version string
		wantErr bool
	}{{"deno 2.2.9 (stable)", true}, {"deno 2.3.0 (stable)", false}, {"deno 2.9.7 (stable)", false}, {"node 22.0.0", true}} {
		t.Run(test.version, func(t *testing.T) {
			if err := checkDenoVersion(test.version); (err != nil) != test.wantErr {
				t.Fatalf("checkDenoVersion(%q) error = %v, wantErr %t", test.version, err, test.wantErr)
			}
		})
	}
}
