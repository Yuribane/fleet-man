package tui

import "testing"

// TestSameSavedGroupEqual confirms byte-identical savedGroups compare equal.
// The diff gate in saveCurrentGroupLayout depends on this being true so
// that idle discovery ticks don't rewrite state.json.
func TestSameSavedGroupEqual(t *testing.T) {
	a := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		Layout:       "4fe2,140x40,0,0{...}",
		PaneCount:    2,
	}
	b := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		Layout:       "4fe2,140x40,0,0{...}",
		PaneCount:    2,
	}
	if !sameSavedGroup(a, b) {
		t.Fatalf("identical savedGroups reported unequal")
	}
}

// TestSameSavedGroupFieldDifferences verifies every scalar field
// participates in the comparison. If any field is dropped from
// sameSavedGroup, the corresponding subtest fails and the poll loop
// silently stops persisting that kind of change.
func TestSameSavedGroupFieldDifferences(t *testing.T) {
	base := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123"},
		Layout:       "L",
		PaneCount:    1,
	}
	cases := []struct {
		name string
		mut  func(*savedGroup)
	}{
		{"GroupID", func(g *savedGroup) { g.GroupID = "other" }},
		{"InstanceName", func(g *savedGroup) { g.InstanceName = "beta" }},
		{"Layout", func(g *savedGroup) { g.Layout = "L2" }},
		{"PaneCount", func(g *savedGroup) { g.PaneCount = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := base
			tc.mut(&b)
			if sameSavedGroup(base, b) {
				t.Fatalf("sameSavedGroup returned true when %s differed", tc.name)
			}
		})
	}
}

// TestSameSavedGroupSessionsDiffer covers slice-content and slice-length
// differences, which the scalar loop above can't express without a
// helper.
func TestSameSavedGroupSessionsDiffer(t *testing.T) {
	base := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		Layout:       "L",
		PaneCount:    2,
	}

	t.Run("different element", func(t *testing.T) {
		b := base
		b.Sessions = []string{"alpha~abc123", "alpha~abc123~aaaa"}
		if sameSavedGroup(base, b) {
			t.Fatal("sameSavedGroup ignored a session content change")
		}
	})

	t.Run("shorter slice", func(t *testing.T) {
		b := base
		b.Sessions = []string{"alpha~abc123"}
		b.PaneCount = 1 // keep scalars aligned — otherwise the scalar check short-circuits
		if sameSavedGroup(base, b) {
			t.Fatal("sameSavedGroup ignored a session length change")
		}
	})

	t.Run("longer slice", func(t *testing.T) {
		b := base
		b.Sessions = []string{"alpha~abc123", "alpha~abc123~ff00", "alpha~abc123~bbbb"}
		b.PaneCount = 3
		if sameSavedGroup(base, b) {
			t.Fatal("sameSavedGroup ignored a session length change")
		}
	})
}

// TestSameSavedGroupNilVsEmpty documents that nil and empty Sessions
// slices compare equal — both have length zero and the content loop
// never runs. saveCurrentGroupLayout only ever assigns non-nil slices,
// so the distinction doesn't matter in practice.
func TestSameSavedGroupNilVsEmpty(t *testing.T) {
	a := savedGroup{GroupID: "g", InstanceName: "i", Sessions: nil, Layout: "L"}
	b := savedGroup{GroupID: "g", InstanceName: "i", Sessions: []string{}, Layout: "L"}
	if !sameSavedGroup(a, b) {
		t.Fatal("nil and empty Sessions slices should compare equal")
	}
}

func TestSavedGroupSessionNamesUsesSavedOrder(t *testing.T) {
	sg := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		PaneCount:    2,
	}

	got := savedGroupSessionNames(sg, "alpha")
	want := []string{"alpha~abc123", "alpha~abc123~ff00"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSavedGroupSessionNamesSynthesizesMissingPanes(t *testing.T) {
	sg := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		PaneCount:    2,
	}

	got := savedGroupSessionNames(sg, "alpha")
	want := []string{"alpha~abc123", "alpha~abc123~restored01"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeSavedGroupSessionsReplacesHostPaneTitles(t *testing.T) {
	got := normalizeSavedGroupSessions(
		[]string{"runner-host", "alpha~abc123~ff00"},
		"alpha",
		"abc123",
	)
	want := []string{"alpha~abc123", "alpha~abc123~ff00"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRestoreSessionNamesTopsUpIncompleteLiveDiscovery(t *testing.T) {
	sg := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		PaneCount:    2,
	}

	got := restoreSessionNames(
		"alpha~abc123\n",
		"alpha~abc123",
		sg.Sessions,
		&sg,
		"alpha",
	)
	want := []string{"alpha~abc123", "alpha~abc123~ff00"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRestoreSessionNamesUsesSavedLayoutOverLiveDiscovery(t *testing.T) {
	sg := savedGroup{
		GroupID:      "abc123",
		InstanceName: "alpha",
		Sessions:     []string{"alpha~abc123", "alpha~abc123~ff00"},
		PaneCount:    2,
	}

	got := restoreSessionNames(
		"alpha~abc123\nalpha~abc123~stale\n",
		"alpha~abc123",
		nil,
		&sg,
		"alpha",
	)
	want := []string{"alpha~abc123", "alpha~abc123~ff00"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
