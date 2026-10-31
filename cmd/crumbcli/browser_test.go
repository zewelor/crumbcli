package main

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, file string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserProfileLocations(t *testing.T) {
	for _, tt := range []struct{ browser, root string }{
		{"brave-origin", "BraveSoftware/Brave-Origin"}, {"brave", "BraveSoftware/Brave-Browser"}, {"chrome", "google-chrome"}, {"chromium", "chromium"},
	} {
		t.Run(tt.browser, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, "custom-config")
			want := filepath.Join(config, tt.root, "Default", "Cookies")
			touch(t, want)
			got, err := profileFile(BrowserOptions{Browser: tt.browser}, home, config)
			if err != nil || got != want {
				t.Fatal("browser default path mismatch")
			}
			custom := filepath.Join(config, tt.root, "Profile 2", "Network", "Cookies")
			touch(t, custom)
			got, err = profileFile(BrowserOptions{Browser: tt.browser, Profile: "Profile 2"}, home, config)
			if err != nil || got != custom {
				t.Fatal("explicit browser profile was not selected")
			}
		})
	}
}

func TestNoProfileFallback(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "chosen")
	touch(t, filepath.Join(dir, "Cookies"))
	touch(t, filepath.Join(dir, "Network", "Cookies"))
	if _, err := profileFile(BrowserOptions{Browser: "brave", Directory: dir}, home, ""); err == nil {
		t.Fatal("ambiguous database selected")
	}
	if _, err := profileFile(BrowserOptions{Browser: "brave-origin", Directory: filepath.Join(home, "missing")}, home, ""); err == nil {
		t.Fatal("missing profile was silently recovered")
	}
}

func TestHomeIsRequiredForProfileDiscovery(t *testing.T) {
	if _, err := profileFile(BrowserOptions{Browser: "brave-origin"}, "", ""); err == nil {
		t.Fatal("missing HOME allowed relative profile discovery")
	}
}
