package library

import (
	"errors"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

// TestStatusCRUD pins 6.3: rows default to "want" and SetStatus round-trips
// the three valid states, rejecting anything else.
func TestStatusCRUD(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("s.txt", &model.Tab{Title: "S"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "want" {
		t.Fatalf("default status = %q, want want", row.Status)
	}
	for _, status := range []string{"learning", "learned", "want"} {
		if err := st.SetStatus(id, status); err != nil {
			t.Fatalf("SetStatus(%q): %v", status, err)
		}
		row, err := st.GetRow(id)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status != status {
			t.Fatalf("status = %q, want %q", row.Status, status)
		}
	}
	// Status also flows through List and Search.
	list, err := st.List()
	if err != nil || list[0].Status != "want" {
		t.Fatalf("List status: %+v, %v", list, err)
	}
	found, err := st.Search("S")
	if err != nil || found[0].Status != "want" {
		t.Fatalf("Search status: %+v, %v", found, err)
	}

	if err := st.SetStatus(id, "bogus"); err == nil {
		t.Fatal("invalid status should be rejected")
	}
	if err := st.SetStatus(999, "want"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetStatus(999) err = %v, want ErrNotFound", err)
	}
}

// TestTagsCRUD pins 6.3: tags attach/detach idempotently, sort
// alphabetically, and orphan tags are pruned from the vocabulary.
func TestTagsCRUD(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("s.txt", &model.Tab{Title: "S"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := st.Import("t.txt", &model.Tab{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tag := range []string{"rock", "classics"} {
		if err := st.AddTag(id, tag); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddTag(id2, "rock"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTag(id, "rock"); err != nil { // idempotent
		t.Fatal(err)
	}

	tags, err := st.TagsFor(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "classics" || tags[1] != "rock" {
		t.Fatalf("TagsFor = %v, want [classics rock]", tags)
	}
	all, err := st.AllTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0] != "classics" || all[1] != "rock" {
		t.Fatalf("AllTags = %v, want [classics rock]", all)
	}

	// Detach from one tab: tag survives via the other.
	if err := st.RemoveTag(id, "rock"); err != nil {
		t.Fatal(err)
	}
	tags, _ = st.TagsFor(id)
	if len(tags) != 1 || tags[0] != "classics" {
		t.Fatalf("TagsFor after remove = %v, want [classics]", tags)
	}
	all, _ = st.AllTags()
	if len(all) != 2 {
		t.Fatalf("AllTags after one remove = %v, want 2", all)
	}

	// Detach from the last tab: orphan pruned.
	if err := st.RemoveTag(id2, "rock"); err != nil {
		t.Fatal(err)
	}
	all, _ = st.AllTags()
	if len(all) != 1 || all[0] != "classics" {
		t.Fatalf("AllTags after pruning = %v, want [classics]", all)
	}

	// Errors.
	if err := st.AddTag(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddTag on missing tab err = %v, want ErrNotFound", err)
	}
	if err := st.AddTag(id, "   "); err == nil {
		t.Fatal("empty tag should be rejected")
	}
	if err := st.RemoveTag(id, "nope"); err == nil {
		t.Fatal("removing a tag the tab does not have should error")
	}
}
