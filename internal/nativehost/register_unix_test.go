//go:build !windows

package nativehost

import (
	"os"
	"testing"
)

func TestInstallStatusUninstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ts := targets()
	if len(ts) == 0 {
		t.Skip("no targets")
	}
	// Pretend Chrome and Brave are installed; Edge is not.
	for _, i := range []int{0, 3} {
		if err := os.MkdirAll(ts[i].root, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	host := "/opt/goidm/idm-host"
	names, err := Install(host, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "Google Chrome" || names[1] != "Brave" {
		t.Fatalf("installed for %v", names)
	}

	for _, st := range Status(host) {
		want := st.Name == "Google Chrome" || st.Name == "Brave"
		if st.Installed != want || st.Current != want {
			t.Errorf("%s: %+v", st.Name, st)
		}
	}
	// A moved app makes the registration stale.
	for _, st := range Status("/somewhere/else/idm-host") {
		if st.Installed && st.Current {
			t.Errorf("%s should be stale", st.Name)
		}
	}

	if err := Uninstall(""); err != nil {
		t.Fatal(err)
	}
	for _, st := range Status(host) {
		if st.Installed {
			t.Errorf("%s still installed", st.Name)
		}
	}
}

func TestInstallFallsBackToChrome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	names, err := Install("/opt/goidm/idm-host", "")
	if err != nil || len(names) != 1 || names[0] != "Google Chrome" {
		t.Fatalf("names=%v err=%v", names, err)
	}
}
