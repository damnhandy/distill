package builder

import "testing"

func TestPlatformRef(t *testing.T) {
	tests := []struct {
		base, platform, want string
	}{
		{"registry.example.com/img:1.0", "linux/amd64", "registry.example.com/img:1.0-linux-amd64"},
		{"registry.example.com/img:1.0", "linux/arm64", "registry.example.com/img:1.0-linux-arm64"},
		{"registry.example.com/img:1.0", "linux/arm/v7", "registry.example.com/img:1.0-linux-arm-v7"},
	}
	for _, tc := range tests {
		got := PlatformRef(tc.base, tc.platform)
		if got != tc.want {
			t.Errorf("PlatformRef(%q, %q) = %q, want %q", tc.base, tc.platform, got, tc.want)
		}
	}
}
