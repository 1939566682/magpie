package routes

import (
	"testing"
	"time"
)

// UrgentUntil is the end of the lunch break: the night, the morning and the
// break all end there, and a balance lapsing in any of them is spent by nobody.
// The morning counts even though the user is back at 09:00 — nobody spends 91
// credits in the ten minutes before they lapse.
func TestUrgentUntilIsTheEndOfTheLunchBreak(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 28, h, m, 0, 0, time.Local) }
	next := func(h, m int) time.Time { return time.Date(2026, 10, 29, h, m, 0, 0, time.Local) }
	for _, c := range []struct {
		now  time.Time
		want time.Time
	}{
		// the night: the lunch break of the day to come
		{at(3, 0), at(13, 30)},
		{at(8, 59), at(13, 30)},
		{at(23, 59), next(13, 30)},
		{next(0, 1), next(13, 30)},
		// the morning, and the lunch break itself
		{at(9, 0), at(13, 30)},
		{at(11, 59), at(13, 30)},
		{at(12, 30), at(13, 30)},
		// the afternoon and the evening: the next break, tomorrow
		{at(13, 30), next(13, 30)},
		{at(18, 0), next(13, 30)},
		{at(22, 0), next(13, 30)},
	} {
		if got := UrgentUntil(c.now); !got.Equal(c.want) {
			t.Errorf("UrgentUntil(%s) = %s, want %s",
				c.now.Format("01-02 15:04"), got.Format("01-02 15:04"), c.want.Format("01-02 15:04"))
		}
	}
}

// ExpireFirst is the routing that weighs what is about to be lost, so it is
// the one whose seats the gateway moves a warm conversation for.
func TestExpireFirstIsUrgentAndTwoDaysWide(t *testing.T) {
	if !ExpireFirst.Urgent {
		t.Fatal("ExpireFirst doesn't say its seats can be urgent")
	}
	if ExpireHorizon != 48*time.Hour {
		t.Fatalf("ExpireHorizon = %s, want 48h", ExpireHorizon)
	}
}
