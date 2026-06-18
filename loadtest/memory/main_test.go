package main

import "testing"

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a, b ,, a ")
	want := []string{"a", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("item %d=%q want %q", index, got[index], want[index])
		}
	}
}

func TestSanitizeRunName(t *testing.T) {
	got := sanitizeRunName("memory smoke: 2026/06/19")
	if got != "memory-smoke--2026-06-19" {
		t.Fatalf("sanitize=%q", got)
	}
}

func TestPathInside(t *testing.T) {
	if !pathInside(`E:\development\IM\docs`, `E:\development\IM`) {
		t.Fatalf("expected docs under repo")
	}
	if pathInside(`H:\NexusIM\loadtest-results\memory`, `E:\development\IM`) {
		t.Fatalf("H drive output must not be treated as repo-local")
	}
}
