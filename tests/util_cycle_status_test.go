package bdtui_test

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

func TestCyclePriorityForwardAndBackward(t *testing.T) {
	t.Parallel()

	forwardCases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 1},
		{in: 1, want: 2},
		{in: 2, want: 3},
		{in: 3, want: 4},
		{in: 4, want: 0},
	}
	for _, tc := range forwardCases {
		if got := cyclePriority(tc.in); got != tc.want {
			t.Fatalf("cyclePriority(%d): got %d, want %d", tc.in, got, tc.want)
		}
	}

	backwardCases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 4},
		{in: 1, want: 0},
		{in: 2, want: 1},
		{in: 3, want: 2},
		{in: 4, want: 3},
	}
	for _, tc := range backwardCases {
		if got := cyclePriorityBackward(tc.in); got != tc.want {
			t.Fatalf("cyclePriorityBackward(%d): got %d, want %d", tc.in, got, tc.want)
		}
	}
}
