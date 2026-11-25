package doctor

import "testing"

func TestCheckMPVVersionRequires041(t *testing.T) {
	for _, tc := range []struct {
		version string
		wantErr bool
	}{{"mpv 0.41.0", false}, {"mpv 0.42.1", false}, {"mpv 0.40.0", true}, {"not mpv", true}} {
		t.Run(tc.version, func(t *testing.T) {
			err := checkMPVVersion(tc.version)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkMPVVersion(%q) error = %v, wantErr %t", tc.version, err, tc.wantErr)
			}
		})
	}
}
