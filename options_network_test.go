package gbash

import (
	"testing"
	"time"
)

func TestWithHTTPAccessKeepsEarlierNetworkSettings(t *testing.T) {
	t.Parallel()
	var cfg Config
	for _, opt := range []Option{
		WithNetwork(&NetworkConfig{
			AllowedURLPrefixes: []string{"https://old.example/"},
			DenyPrivateRanges:  true,
			Timeout:            3 * time.Second,
			MaxResponseBytes:   42,
		}),
		WithHTTPAccess("https://api.example/"),
	} {
		if err := opt(&cfg); err != nil {
			t.Fatalf("option error = %v", err)
		}
	}
	got := cfg.Network
	if !got.DenyPrivateRanges || got.Timeout != 3*time.Second || got.MaxResponseBytes != 42 {
		t.Fatalf("Network = %+v, want earlier settings kept", got)
	}
	if len(got.AllowedURLPrefixes) != 1 || got.AllowedURLPrefixes[0] != "https://api.example/" {
		t.Fatalf("AllowedURLPrefixes = %v", got.AllowedURLPrefixes)
	}
}
