package library

import (
	"errors"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

// TestSetlistCRUD pins 6.3: creation, append order, reorder, removal with
// renumbering, and the error paths.
func TestSetlistCRUD(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.Import("a.txt", &model.Tab{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.Import("b.txt", &model.Tab{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.Import("c.txt", &model.Tab{Title: "C"})
	if err != nil {
		t.Fatal(err)
	}

	slID, err := st.CreateSetlist("  Practice Set  ")
	if err != nil {
		t.Fatal(err)
	}
	if slID <= 0 {
		t.Fatalf("setlist id = %d, want positive", slID)
	}

	for _, tab := range []int64{a, b, c} {
		if err := st.AddToSetlist(slID, tab); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddToSetlist(slID, a); err != nil { // no-op duplicate
		t.Fatal(err)
	}

	lists, err := st.Setlists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Name != "Practice Set" || lists[0].TabCount != 3 {
		t.Fatalf("Setlists = %+v, want one setlist with 3 tabs", lists)
	}

	tabs, err := st.SetlistTabs(slID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tabs) != 3 || tabs[0].ID != a || tabs[1].ID != b || tabs[2].ID != c {
		t.Fatalf("SetlistTabs order = [%d %d %d], want [%d %d %d]",
			tabs[0].ID, tabs[1].ID, tabs[2].ID, a, b, c)
	}

	// Reorder: full list.
	if err := st.ReorderSetlist(slID, []int64{c, a, b}); err != nil {
		t.Fatal(err)
	}
	tabs, _ = st.SetlistTabs(slID)
	if tabs[0].ID != c || tabs[1].ID != a || tabs[2].ID != b {
		t.Fatalf("after reorder = [%d %d %d], want [%d %d %d]",
			tabs[0].ID, tabs[1].ID, tabs[2].ID, c, a, b)
	}

	// Reorder: partial list keeps unlisted members after, in relative order.
	if err := st.ReorderSetlist(slID, []int64{b}); err != nil {
		t.Fatal(err)
	}
	tabs, _ = st.SetlistTabs(slID)
	if tabs[0].ID != b || tabs[1].ID != c || tabs[2].ID != a {
		t.Fatalf("after partial reorder = [%d %d %d], want [%d %d %d]",
			tabs[0].ID, tabs[1].ID, tabs[2].ID, b, c, a)
	}

	// Removal renumbers the tail.
	if err := st.RemoveFromSetlist(slID, c); err != nil {
		t.Fatal(err)
	}
	tabs, _ = st.SetlistTabs(slID)
	if len(tabs) != 2 || tabs[0].ID != b || tabs[1].ID != a {
		t.Fatalf("after remove = [%d %d], want [%d %d]", tabs[0].ID, tabs[1].ID, b, a)
	}
	lists, _ = st.Setlists()
	if lists[0].TabCount != 2 {
		t.Fatalf("TabCount = %d, want 2", lists[0].TabCount)
	}
	if err := st.RemoveFromSetlist(slID, c); err != nil { // non-member: no-op
		t.Fatalf("removing a non-member should be a no-op, got %v", err)
	}

	// Error paths.
	if _, err := st.CreateSetlist("   "); err == nil {
		t.Fatal("empty setlist name should be rejected")
	}
	if err := st.AddToSetlist(999, a); !errors.Is(err, ErrSetlistNotFound) {
		t.Fatalf("AddToSetlist(999) err = %v, want ErrSetlistNotFound", err)
	}
	if err := st.AddToSetlist(slID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddToSetlist(missing tab) err = %v, want ErrNotFound", err)
	}
	if _, err := st.SetlistTabs(999); !errors.Is(err, ErrSetlistNotFound) {
		t.Fatalf("SetlistTabs(999) err = %v, want ErrSetlistNotFound", err)
	}
	if err := st.ReorderSetlist(999, nil); !errors.Is(err, ErrSetlistNotFound) {
		t.Fatalf("ReorderSetlist(999) err = %v, want ErrSetlistNotFound", err)
	}
}

// TestSetlistEmptyAndMultiple: several setlists coexist and empty setlists
// are listed with zero tabs.
func TestSetlistEmptyAndMultiple(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tab, err := st.Import("a.txt", &model.Tab{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	sl1, err := st.CreateSetlist("Full")
	if err != nil {
		t.Fatal(err)
	}
	sl2, err := st.CreateSetlist("Empty")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddToSetlist(sl1, tab); err != nil {
		t.Fatal(err)
	}
	lists, err := st.Setlists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 {
		t.Fatalf("Setlists = %+v, want 2", lists)
	}
	byName := map[string]int{}
	for _, l := range lists {
		byName[l.Name] = l.TabCount
	}
	if byName["Full"] != 1 || byName["Empty"] != 0 {
		t.Fatalf("setlist counts = %+v, want Full=1 Empty=0", byName)
	}
	empty, err := st.SetlistTabs(sl2)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty setlist tabs = %+v, want none", empty)
	}
}
