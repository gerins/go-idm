package engine

import "testing"

// Two downloads choosing a name at the same moment must not get the same one.
// Each job holds its own copy of the download, so the first claim has to be
// visible to the second through the shared entry before claimPath returns.
func TestClaimPathIsVisibleToOtherJobsImmediately(t *testing.T) {
	m, _, _ := newTestManager(t, nil)
	a := mustAdd(t, m, AddRequest{URL: "http://127.0.0.1:1/a.bin", StartPaused: true})
	b := mustAdd(t, m, AddRequest{URL: "http://127.0.0.1:1/b.bin", StartPaused: true})

	jobFor := func(id string) *job {
		m.mu.Lock()
		defer m.mu.Unlock()
		e := m.entries[id]
		return &job{m: m, e: e, d: cloneDownload(&e.d)}
	}
	ja, jb := jobFor(a.ID), jobFor(b.ID)
	if err := m.claimPath(ja, "report.bin"); err != nil {
		t.Fatal(err)
	}
	// No commit in between: this is the window the race lived in.
	if err := m.claimPath(jb, "report.bin"); err != nil {
		t.Fatal(err)
	}
	if ja.d.FileName == jb.d.FileName {
		t.Fatalf("both downloads claimed %q", ja.d.FileName)
	}
}
