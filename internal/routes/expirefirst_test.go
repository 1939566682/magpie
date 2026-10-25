package routes

import (
	"slices"
	"testing"
	"time"
)

// ExpireFirst spends what is about to be lost, then what costs nothing, then
// the rest by how soon their credits run out: seats whose credits run out
// within ExpireHorizon first (soonest first), the free ones next in their
// listed order, the rest by their expiry (one that never expires last), and
// one at 98% or more of its allowance behind all of them.
func TestExpireFirstOrdersLapsingThenFreeThenSoonest(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) time.Time { return now.Add(d) }
	cases := []struct {
		name  string
		seats []Seat
		want  []int
	}{
		{
			// b's credits run out in two hours: it goes first, ahead of a
			// free seat and one that expires in thirty days
			name: "what lapses goes first",
			seats: []Seat{
				{Key: "free", Free: true},
				{Key: "lapsing", Soon: []time.Time{at(2 * time.Hour)}},
				{Key: "later", Soon: []time.Time{at(30 * 24 * time.Hour)}},
			},
			want: []int{1, 0, 2},
		},
		{
			// nothing about to lapse: the free tier answers
			name: "nothing lapsing, free first",
			seats: []Seat{
				{Key: "paid", Soon: []time.Time{at(72 * time.Hour)}},
				{Key: "free", Free: true},
			},
			want: []int{1, 0},
		},
		{
			// two about to lapse: the soonest first, both ahead of a free one
			name: "the soonest of two lapsing",
			seats: []Seat{
				{Key: "five", Soon: []time.Time{at(5 * time.Hour)}},
				{Key: "one", Soon: []time.Time{at(time.Hour)}},
				{Key: "free", Free: true},
			},
			want: []int{1, 0, 2},
		},
		{
			// nothing lapsing and nothing free: the soonest expiry first,
			// one that never expires last
			name: "the rest by soonest expiry",
			seats: []Seat{
				{Key: "never"},
				{Key: "ten", Soon: []time.Time{at(10 * 24 * time.Hour)}},
				{Key: "three", Soon: []time.Time{at(3 * 24 * time.Hour)}},
			},
			want: []int{2, 1, 0},
		},
		{
			// 95% used is not spent: lapsing still takes the request
			name: "lapsing at 95% still goes first",
			seats: []Seat{
				{Key: "free", Free: true},
				{Key: "lapsing", Used: 95, Soon: []time.Time{at(time.Hour)}},
			},
			want: []int{1, 0},
		},
		{
			// 98% or more is spent: it waits even about to lapse
			name: "spent waits even lapsing",
			seats: []Seat{
				{Key: "spent", Used: 99, Soon: []time.Time{at(time.Hour)}},
				{Key: "free", Free: true},
			},
			want: []int{1, 0},
		},
		{
			// a seat's own windows come in any order: the soonest counts
			name: "soonest of a seat's own windows",
			seats: []Seat{
				{Key: "a", Soon: []time.Time{at(48 * time.Hour), at(3 * time.Hour)}},
				{Key: "b", Soon: []time.Time{at(6 * time.Hour)}},
			},
			want: []int{0, 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Apply(ExpireFirst, "group/g", c.seats); !slices.Equal(got, c.want) {
				t.Fatalf("order = %v, want %v", got, c.want)
			}
		})
	}
}

// ExpireFirst asks the gateway for Free too, since what costs nothing is its
// second tier.
func TestExpireFirstIsByFreeAndBuiltin(t *testing.T) {
	if !ExpireFirst.ByFree {
		t.Fatal("ExpireFirst doesn't ask the gateway for Free")
	}
	s, ok := Builtin["expire-first"]
	if !ok {
		t.Fatal("\"expire-first\" is not among Builtin")
	}
	if s.Order == nil {
		t.Fatal("\"expire-first\" has no order")
	}
}
