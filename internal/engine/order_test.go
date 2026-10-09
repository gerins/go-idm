package engine

import (
	"slices"
	"testing"
	"time"
)

func ids(infos []Info) []string {
	out := make([]string, len(infos))
	for i, in := range infos {
		out[i] = in.ID
	}
	return out
}

// addPaused adds n paused downloads, which keeps the engine from touching the network.
func addPaused(t *testing.T, m *Manager, n int) []string {
	t.Helper()
	var out []string
	for range n {
		out = append(out, mustAdd(t, m, AddRequest{URL: "http://127.0.0.1:1/f.bin", StartPaused: true}).ID)
	}
	return out
}

func TestMoveReordersQueue(t *testing.T) {
	m, _, _ := newTestManager(t, nil)
	a := addPaused(t, m, 4) // a0 a1 a2 a3

	check := func(want ...string) {
		t.Helper()
		if got := ids(m.List()); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		// The UI sorts by Order, so it must agree with the list.
		infos := m.List()
		for i := 1; i < len(infos); i++ {
			if infos[i].Order <= infos[i-1].Order {
				t.Fatalf("Order not increasing: %v then %v", infos[i-1].Order, infos[i].Order)
			}
		}
	}
	check(a[0], a[1], a[2], a[3])

	if err := m.Move(a[3], a[0]); err != nil { // to the front
		t.Fatal(err)
	}
	check(a[3], a[0], a[1], a[2])

	if err := m.Move(a[3], ""); err != nil { // to the end
		t.Fatal(err)
	}
	check(a[0], a[1], a[2], a[3])

	if err := m.Move(a[0], a[3]); err != nil { // into the middle
		t.Fatal(err)
	}
	check(a[1], a[2], a[0], a[3])

	// Moving next to where it already is changes nothing.
	for _, c := range [][2]string{{a[1], a[2]}, {a[1], a[1]}, {a[3], ""}} {
		if err := m.Move(c[0], c[1]); err != nil {
			t.Fatal(err)
		}
	}
	check(a[1], a[2], a[0], a[3])
}

func TestMoveErrors(t *testing.T) {
	m, _, _ := newTestManager(t, nil)
	a := addPaused(t, m, 2)
	if err := m.Move("nope", a[0]); err != ErrNotFound {
		t.Errorf("unknown id: %v", err)
	}
	if err := m.Move(a[0], "nope"); err != ErrNotFound {
		t.Errorf("unknown target: %v", err)
	}
	if got := ids(m.List()); !slices.Equal(got, a) {
		t.Errorf("failed moves changed the order: %v", got)
	}
}

func TestMoveSurvivesRestart(t *testing.T) {
	st, dir := newMemStore(), t.TempDir()
	m1, _, _ := newTestManagerWith(t, st, dir, nil)
	a := addPaused(t, m1, 3)
	if err := m1.Move(a[2], a[0]); err != nil {
		t.Fatal(err)
	}
	want := ids(m1.List())
	m1.Close()

	m2, _, _ := newTestManagerWith(t, st, dir, nil)
	if got := ids(m2.List()); !slices.Equal(got, want) {
		t.Fatalf("after restart: %v, want %v", got, want)
	}
	// New downloads still go to the end.
	n := addPaused(t, m2, 1)[0]
	if got := ids(m2.List()); got[len(got)-1] != n {
		t.Fatalf("new download not last: %v", got)
	}
}

// Squeezing a download between the same two neighbours over and over uses up
// the gap between their positions; the queue must renumber and stay correct.
func TestMoveRenumbersWhenGapIsExhausted(t *testing.T) {
	st, dir := newMemStore(), t.TempDir()
	m, _, _ := newTestManagerWith(t, st, dir, nil)
	a := addPaused(t, m, 4)
	x, y, z := a[0], a[1], a[2]
	// Repeatedly put z just before y, then x just before z, so each lands in a
	// gap half the size of the last.
	for range 80 {
		if err := m.Move(z, y); err != nil {
			t.Fatal(err)
		}
		if err := m.Move(x, z); err != nil {
			t.Fatal(err)
		}
		if err := m.Move(y, x); err != nil {
			t.Fatal(err)
		}
	}
	want := ids(m.List())
	infos := m.List()
	for i := 1; i < len(infos); i++ {
		if infos[i].Order <= infos[i-1].Order {
			t.Fatalf("Order not increasing at %d: %v", i, infos)
		}
	}
	m.Close()

	m2, _, _ := newTestManagerWith(t, st, dir, nil)
	if got := ids(m2.List()); !slices.Equal(got, want) {
		t.Fatalf("after restart: %v, want %v", got, want)
	}
}

// Downloads saved before queue order existed come back in creation order.
func TestLoadNumbersLegacyDownloads(t *testing.T) {
	st := newMemStore()
	base := time.Now().Add(-time.Hour)
	for i, id := range []string{"c", "a", "b"} { // created c, a, b; none has an Order
		_ = st.Save(&Download{ID: id, URL: "http://127.0.0.1:1/f.bin", Status: StatusPaused, CreatedAt: base.Add(time.Duration(i) * time.Minute)})
	}
	m, _, _ := newTestManagerWith(t, st, t.TempDir(), nil)
	if got := ids(m.List()); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("order = %v", got)
	}
	// And the numbering was saved, so a later load does not depend on timestamps again.
	for id, want := range map[string]float64{"c": 1, "a": 2, "b": 3} {
		if got := st.m[id].Order; got != want {
			t.Errorf("saved Order of %s = %v, want %v", id, got, want)
		}
	}
}

// Moving a queued download up makes it start before the ones it jumped over.
func TestMoveChangesStartOrder(t *testing.T) {
	data := randomData(512 << 10)
	srv := newTestServer(t, data, 10*time.Millisecond)
	m, _, _ := newTestManager(t, func(c *Config) { c.MaxActive = 1; c.Connections = 1 })

	first := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "first.bin"})
	second := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "second.bin"})
	third := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "third.bin"})
	waitFor(t, m, first.ID, func(i Info) bool { return i.Downloaded > 0 }, "first running")

	if err := m.Move(third.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, first.ID, status(StatusCompleted), "first done")
	waitFor(t, m, third.ID, func(i Info) bool { return i.Status != StatusQueued }, "third starts next")
	if s, _ := m.Get(second.ID); s.Status != StatusQueued {
		t.Fatalf("second should still be waiting, got %s", s.Status)
	}
}
