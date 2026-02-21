package main

import "testing"

func TestCycleStatusForward(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Status
		want Status
	}{
		{name: "open", in: StatusOpen, want: StatusInProgress},
		{name: "in progress", in: StatusInProgress, want: StatusBlocked},
		{name: "blocked", in: StatusBlocked, want: StatusClosed},
		{name: "closed", in: StatusClosed, want: StatusOpen},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cycleStatus(tc.in); got != tc.want {
				t.Fatalf("cycleStatus(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCycleStatusBackward(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Status
		want Status
	}{
		{name: "open", in: StatusOpen, want: StatusClosed},
		{name: "in progress", in: StatusInProgress, want: StatusOpen},
		{name: "blocked", in: StatusBlocked, want: StatusInProgress},
		{name: "closed", in: StatusClosed, want: StatusBlocked},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cycleStatusBackward(tc.in); got != tc.want {
				t.Fatalf("cycleStatusBackward(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
