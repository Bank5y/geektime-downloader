package browsercookie

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfilesAndCookieDB(t *testing.T) {
	dir := t.TempDir()
	state := `{"profile":{"last_used":"Profile 2","info_cache":{
		"Default":{"name":"Personal","active_time":100},
		"Profile 1":{"name":"Work","active_time":300},
		"Profile 2":{"name":"Last","active_time":1}}}}`
	if err := os.WriteFile(filepath.Join(dir, "Local State"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	ps := profiles(dir)
	if len(ps) != 3 || ps[0].dir != "Profile 2" || ps[1].dir != "Profile 1" || ps[2].name != "Personal" {
		t.Fatalf("unexpected order: %+v", ps)
	}

	if got := profiles(t.TempDir()); len(got) != 1 || got[0].dir != "Default" {
		t.Fatalf("fallback: %+v", got)
	}

	p := filepath.Join(dir, "Default")
	if cookieDB(p) != "" {
		t.Fatal("no db yet")
	}
	_ = os.MkdirAll(p, 0o700)
	_ = os.WriteFile(filepath.Join(p, "Cookies"), nil, 0o600)
	if cookieDB(p) != "Cookies" {
		t.Fatal("legacy location")
	}
	_ = os.MkdirAll(filepath.Join(p, "Network"), 0o700)
	_ = os.WriteFile(filepath.Join(p, "Network", "Cookies"), nil, 0o600)
	if cookieDB(p) != filepath.Join("Network", "Cookies") {
		t.Fatal("network location should win")
	}
}
