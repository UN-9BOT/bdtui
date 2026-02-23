package main

import (
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestBeadsWatchTargets(t *testing.T) {
	t.Parallel()

	root := "/tmp/repo/.beads"
	got := beadsWatchTargets(root)
	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(got))
	}
	if got[0] != filepath.Join(root, "last-touched") {
		t.Fatalf("unexpected first target: %q", got[0])
	}
	if got[1] != filepath.Join(root, "issues.jsonl") {
		t.Fatalf("unexpected second target: %q", got[1])
	}
	if got[2] != root {
		t.Fatalf("unexpected third target: %q", got[2])
	}
}

func TestIsBeadsWatchEventRelevant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ev   fsnotify.Event
		want bool
	}{
		{
			name: "last touched write",
			ev: fsnotify.Event{
				Name: "/repo/.beads/last-touched",
				Op:   fsnotify.Write,
			},
			want: true,
		},
		{
			name: "issues create",
			ev: fsnotify.Event{
				Name: "/repo/.beads/issues.jsonl",
				Op:   fsnotify.Create,
			},
			want: true,
		},
		{
			name: "other file write",
			ev: fsnotify.Event{
				Name: "/repo/.beads/config.yaml",
				Op:   fsnotify.Write,
			},
			want: false,
		},
		{
			name: "chmod last touched",
			ev: fsnotify.Event{
				Name: "/repo/.beads/last-touched",
				Op:   fsnotify.Chmod,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isBeadsWatchEventRelevant(tc.ev)
			if got != tc.want {
				t.Fatalf("isBeadsWatchEventRelevant()=%v want=%v for event=%+v", got, tc.want, tc.ev)
			}
		})
	}
}
