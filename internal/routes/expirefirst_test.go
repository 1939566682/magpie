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
			// a due already past is a reading gone old, not a balance about
			// to go: it must not hold the front that what really lapses has
			name: "a past due is no urgency",
			seats: []Seat{
				{Key: "stale", Soon: []time.Time{at(-3 * time.Hour)}},
				{Key: "soon", Soon: []time.Time{at(2 * time.Hour)}},
				{Key: "free", Free: true},
			},
			want: []int{1, 2, 0},
		},
		{
			// a model that costs nothing spends none of the balance under it:
			// paid credits about to lapse go first, whatever a free model's
			// own reading says it loses at midnight
			name: "a free model's lapsing balance is no loss",
			seats: []Seat{
				{Key: "free", Free: true, Soon: []time.Time{at(time.Hour)}},
				{Key: "paid", Soon: []time.Time{at(9 * time.Hour)}},
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

// The check-in credits (tierTwo: AutoClaw, MiniMax, Qoder, Kimi Work) go
// after what lapses within the horizon and before what merely costs
// nothing: a balance with no known day may be gone tomorrow while a free
// model stays free. A free model of one of them stays among the free seats.
// A reading is not required: a vendor whose usage read is stale or failed is
// still that vendor's balance, and is spent until its vendor refuses it.
// One all but used up waits behind everything, whatever vendor it is.
func TestExpireFirstSpendsWhatNamesNoDay(t *testing.T) {
	now := time.Now()
	soon, far := now.Add(9*time.Hour), now.Add(20*24*time.Hour)
	seats := []Seat{
		{Key: "free", Known: true, Used: 0, Free: true, Soon: []time.Time{far}},
		{Key: "autoclaw-cn@187****1926", Known: true, Used: 0},                               // 签到积分：有额度、无到期
		{Key: "lapsing", Known: true, Used: 0, Soon: []time.Time{soon}},                      // 24h 内过期
		{Key: "nvidia/nvidia/nemotron-3-ultra-550b-a55b", Known: false, Used: 0, Free: true}, // 免费 API key：读不到用量
		{Key: "kimi-work", Known: true, Used: 0, Soon: []time.Time{far}},                     // 共享额度：本月无重置信息
		{Key: "autoclaw-cn@187****1926", Known: false, Used: 0},                              // 同族但用量没读到：照样提前
		{Key: "spent", Known: true, Used: 100},                                               // 已用尽
	}
	got := ExpireFirst.Order("group/free-credit", seats)
	// 第 2 级内部不排先后，保持池里的列出顺序（两条 autoclaw 都在 kimi 前后按池序）；
	// 第 3 级里免费席位按原有顺序，nvidia 不再被提前
	want := []string{"lapsing", "autoclaw-cn@187****1926", "kimi-work", "autoclaw-cn@187****1926", "free", "nvidia/nvidia/nemotron-3-ultra-550b-a55b", "spent"}
	for i, k := range want {
		if seats[got[i]].Key != k {
			t.Fatalf("seat %d is %s, want %s (order: %v)", i, seats[got[i]].Key, k, keysOf(seats, got))
		}
	}
	// a spent check-in credit waits too: used up outranks everything
	spent := []Seat{{Key: "qoder-cn@aliyun0812663101", Known: true, Used: 100}, {Key: "free", Known: true, Used: 0, Free: true, Soon: []time.Time{far}}}
	if got := ExpireFirst.Order("g", spent); spent[got[0]].Key != "free" {
		t.Fatalf("a used-up check-in credit went first: %v", keysOf(spent, got))
	}
}

func keysOf(seats []Seat, idx []int) []string {
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, seats[i].Key)
	}
	return out
}

// A free model a tierTwo vendor lists is not a balance: it stays with the
// other free seats (rank 3), behind the paid seats of those vendors. The
// vendor id alone does not make a seat rank 2 — its model has to cost
// something on that seat.
func TestExpireFirstFreeModelOfTierTwoStaysFree(t *testing.T) {
	seats := []Seat{
		{Key: "qo" + "der-cn@aliyun0812663101/qfmodel", Known: true, Used: 0, Free: true},
		{Key: "qo" + "der-cn@aliyun0812663101", Known: true, Used: 18},
		{Key: "openrouter/x:free", Known: true, Used: 0, Free: true},
	}
	got := ExpireFirst.Order("g", seats)
	if seats[got[0]].Key != "qo"+"der-cn@aliyun0812663101" {
		t.Fatalf("the paid pool went after the free models: %v", keysOf(seats, got))
	}
}
