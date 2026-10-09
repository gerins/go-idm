//go:build !windows

package nativehost

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestInstallFirefox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var ff target
	for _, tg := range targets() {
		if tg.firefox {
			ff = tg
		}
	}
	if ff.name == "" {
		t.Fatal("no Firefox target")
	}
	if err := os.MkdirAll(ff.root, 0o755); err != nil {
		t.Fatal(err)
	}

	host := "/opt/goidm/idm-host"
	names, err := Install(host, "")
	if err != nil || len(names) != 1 || names[0] != "Firefox" {
		t.Fatalf("names=%v err=%v", names, err)
	}
	data, err := os.ReadFile(ff.manifestPath())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := FirefoxManifest(host)
	if string(data) != string(want) {
		t.Errorf("manifest = %s", data)
	}
	if runtime.GOOS == "linux" && ff.hostDir() != filepath.Join(home, ".mozilla", "native-messaging-hosts") {
		t.Errorf("host dir = %s", ff.hostDir())
	}
	for _, st := range Status(host) {
		if want := st.Name == "Firefox"; st.Installed != want || st.Current != want {
			t.Errorf("%s: %+v", st.Name, st)
		}
	}
	if err := Uninstall(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ff.manifestPath()); !os.IsNotExist(err) {
		t.Errorf("manifest still present: %v", err)
	}
}
