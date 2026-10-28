package routes

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// Shares of an allowance past which an account is kept for when the others
// can't take a request: low, and all but used up — where the agent signed
// in to it is signed in to another, too (provider.SpentShare).
const (
	LowShare  = 90
	UsedShare = provider.SpentShare
)

// tier is 0 for a share with room, 1 for low, 2 for all but used up.
// tierTwo are the vendors whose credits name no day of their own: the
// daily check-in balances (AutoClaw's, MiniMax's, Qoder's) and Kimi Work's
// shared monthly allowance. A credit with no known day may be gone
// tomorrow while a free model stays free, so ExpireFirst spends them after
// what lapses within the horizon and before what merely costs nothing.
// They are named, not inferred: a free API key with no usage read is not
// one of them. Take a vendor off the list when its upstream starts telling
// a day — it then ranks by that day with everyone else.
var tierTwo = []string{"autoclaw-cn", "minimax-code", "qoder-cn", "kimi-work"}

// unmet says whether the seat is a balance to spend: one of those vendors,
// whose model costs something. Free is excluded — these vendors list free
// models beside their paid ones (Qoder's qfmodel / dfmodel stand with
// gfmodel), and a free one spends no credit, so it is not the point of this
// tier and stays among the free seats (rank 3).
//
// A reading in hand is not required. It was, so that a vendor whose usage
// read failed would not be sent requests that only fail: the backoff ladder.
// The price was the opposite fault — a vendor whose read stayed broken sat
// behind every free model for as long as it stayed broken (kimi-work's
// reading cached from 10-25, 10-28), which is the unused balance this tier
// exists to spend. A seat here is spent until its vendor refuses it, and the
// refusal rests it by the time it names (restAfter) — the reset its reading
// would have given anyway. The key names the provider before the account
// ("id@user") or the model ("id/model") — and, for a member of a group, after
// the group's own id grown on it ("group/<id>/provider@user"), so it is
// looked for in every segment and not only at the start: read at the start
// alone, no seat of a group's members ever matched, and this tier never ran.
func unmet(s Seat) bool {
	if s.Free {
		return false
	}
	for _, part := range strings.FieldsFunc(s.Key, func(r rune) bool { return r == '/' || r == '@' }) {
		for _, v := range tierTwo {
			if part == v {
				return true
			}
		}
	}
	return false
}

func tier(share float64) int {
	switch {
	case share >= UsedShare:
		return 2
	case share >= LowShare:
		return 1
	}
	return 0
}

func identity(n int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return idx
}

// Smart is the default: of those with quota to spare, the one whose
// allowance renews soonest, since what it has left is lost then, while one
// renewing later keeps: the biggest window decides — the week, not the five
// hours in it — and the next one only when that renews in the same hour. An
// account with no week (Claude Enterprise's five hours alone) goes by its
// five hours, so ahead of every week but one renewing sooner; a window not
// started renews its whole span from now (#576). A Codex account that
// spends a reset about to run out by itself (auto-use on, its windows used)
// renews when that is spent, if sooner: what it has left is lost there just
// the same (#718). Only holding one, or one that runs out after the week
// renews, changes nothing. Those alike stay in their order, keeping the
// vendor's prompt cache warm. Past that, whichever has the most left, and
// one all but used up only when nothing else can take it, one that goes
// on on its credits first of those (spentBefore). Only the windows
// that count the model do: Opus's own weekly allowance being used up leaves
// Sonnet alone.
//
// One not known that tells what it has left as it answers goes first,
// once: else, kept after those known, it would never answer and never be
// known (Anthropic's usage endpoint can turn magpie away for hours).
var Smart = Strategy{Order: func(_ string, seats []Seat) []int {
	var fine, low, spent []int
	for i, s := range seats {
		switch tier(s.Used) {
		case 2:
			spent = append(spent, i)
		case 1:
			low = append(low, i)
		default:
			fine = append(fine, i)
		}
	}
	sort.SliceStable(fine, func(i, j int) bool {
		a, b := seats[fine[i]], seats[fine[j]]
		if a.Learns != b.Learns {
			return a.Learns
		}
		ri, rj := a.Soon, b.Soon
		for k := 0; k < len(ri) || k < len(rj); k++ {
			var x, y time.Time // to the hour, so a few minutes don't reorder
			if k < len(ri) {
				x = ri[k].Truncate(time.Hour)
			}
			if k < len(rj) {
				y = rj[k].Truncate(time.Hour)
			}
			switch {
			case x.Equal(y):
				continue
			case x.IsZero() || y.IsZero(): // not known: after those known
				return y.IsZero()
			}
			return x.Before(y)
		}
		return false
	})
	sort.SliceStable(low, func(i, j int) bool { return seats[low[i]].Used < seats[low[j]].Used })
	sort.SliceStable(spent, func(i, j int) bool { return spentBefore(seats[spent[i]], seats[spent[j]]) })
	return append(append(fine, low...), spent...)
}}

// spentBefore orders two seats all but used up: one that goes on on its
// credits first (a Codex account holding them answers past its allowance;
// one with nothing left is refused), then the one with the most left.
func spentBefore(a, b Seat) bool {
	if a.Credits != b.Credits {
		return a.Credits
	}
	return a.Used < b.Used
}

// Ordered tries a pool in its order: the first until it can't answer.
var Ordered = Strategy{Exact: true, Order: func(_ string, seats []Seat) []int { return identity(len(seats)) }}

var turns = struct {
	sync.Mutex
	m map[string]int // pool → requests routed in turn
}{m: map[string]int{}}

// Rotate starts each request one further along the pool.
var Rotate = Strategy{Order: func(pool string, seats []Seat) []int {
	turns.Lock()
	n := turns.m[pool] % len(seats)
	turns.m[pool]++
	turns.Unlock()
	return append(identity(len(seats))[n:], identity(n)...)
}}

// LeastUsed puts a subscription, or a key that reads its own windows, by the
// share of its allowance used, as the vendor says; then, and for other
// keys, by what magpie sent it lately. Of those all but used up, one that
// goes on on its credits goes first (spentBefore).
var LeastUsed = Strategy{ByUse: true, Order: func(_ string, seats []Seat) []int {
	idx := identity(len(seats))
	sort.SliceStable(idx, func(a, b int) bool {
		sa, sb := seats[idx[a]], seats[idx[b]]
		if ta, tb := tier(sa.Used), tier(sb.Used); ta == 2 && tb == 2 && sa.Credits != sb.Credits {
			return sa.Credits
		}
		if sa.Used != sb.Used {
			return sa.Used < sb.Used
		}
		return sa.Tokens < sb.Tokens
	})
	return idx
}}

// Pace puts a subscription by what it has left of its week per hour until
// that renews, the most first: it has the most to lose at the reset, where
// the most left alone (least used) sends a fresh account ahead of one with
// 80% left and an hour to go, and the soonest reset alone (smart) sends one
// with 3% left and an hour to go ahead of one with 90% and two. The tiers
// are Smart's: one at 90% or more of any window that counts the model — the
// five hours too, a rate cap it would be sent into before its allowance was
// read again — waits until the others can't answer, one all but used up
// until nothing else can; each tier by the share used, one all but used up
// that goes on on its credits first (spentBefore). Among the rest, one
// not known that tells what it has left as it answers goes first while it
// isn't, as in Smart, else it would never be known; then the pace, those
// alike within a tenth by what magpie sent them lately, then in their
// order, keeping the vendor's prompt cache warm. An account not known
// counts as a whole week ahead of it; a key has no week, unless it reads
// its own windows (a sub2api key's limits). A Codex account that spends a
// reset about to run out by itself goes by the hours until then when that
// is sooner (Allowance.Pace, #717).
var Pace = Strategy{ByUse: true, Order: func(_ string, seats []Seat) []int {
	// the bands of pace alike within a tenth, the highest first: drawn
	// before sorting, from each band's highest down, so that sorting by
	// them is consistent — "within a tenth of each other" alone is not,
	// three paces a twelfth apart each going round in a circle. Drawn among
	// those the pace orders alone — one running low would otherwise set a
	// band's top and part two alike behind it
	var byPace []int
	for i, s := range seats {
		if tier(s.Used) == 0 && !s.Learns {
			byPace = append(byPace, i)
		}
	}
	sort.SliceStable(byPace, func(a, b int) bool { return seats[byPace[a]].Pace > seats[byPace[b]].Pace })
	band, top := make([]int, len(seats)), 0.0
	for k, i := range byPace {
		if p := seats[i].Pace; k == 0 || p < 0.9*top {
			top = p
			if k > 0 {
				band[i] = band[byPace[k-1]] + 1
				continue
			}
		} else {
			band[i] = band[byPace[k-1]]
		}
	}
	idx := identity(len(seats))
	sort.SliceStable(idx, func(a, b int) bool {
		sa, sb := seats[idx[a]], seats[idx[b]]
		if ta, tb := tier(sa.Used), tier(sb.Used); ta != tb {
			return ta < tb
		} else if ta == 2 && sa.Credits != sb.Credits {
			return sa.Credits
		} else if ta > 0 {
			if sa.Used != sb.Used {
				return sa.Used < sb.Used
			}
		} else if sa.Learns != sb.Learns {
			return sa.Learns
		} else if band[idx[a]] != band[idx[b]] {
			return band[idx[a]] < band[idx[b]]
		}
		return sa.Tokens < sb.Tokens
	})
	return idx
}}

// Weighted spreads a pool's requests over its seats by each one's weight,
// smoothly (Round): the one the pool's round picks now first, the others
// after it in their order, for failover.
var Weighted = Strategy{Order: func(pool string, seats []Seat) []int {
	keys, weights, rests := make([]string, len(seats)), make([]int, len(seats)), make([]bool, len(seats))
	for i, s := range seats {
		keys[i], weights[i], rests[i] = s.Key, max(s.Weight, 1), s.Resting
	}
	return First(Round(pool, keys, weights, rests), len(seats))
}}

// First is the order of n with best first and the others after it in their
// order; best below 1 leaves it as it is.
func First(best, n int) []int {
	idx := identity(n)
	if best <= 0 {
		return idx
	}
	return append(append([]int{best}, idx[:best]...), idx[best+1:]...)
}

// FreeFirst spends what costs nothing before what is paid for: the seats
// whose model its vendor serves at no cost to the subscription's allowance
// or credits go first, in their listed order, and the rest after them, also
// in their listed order. So a request whose model is free somewhere on the
// pool uses that first, and only when those can't answer does it go to one
// that spends credits or quota — nothing is left unused that cost nothing.
//
// It orders one tier only, unlike Smart: a free seat is not pushed behind a
// paid one for running low, since the free one spends nothing by running
// out of. Among the paid, the listed order stands rather than Smart's
// renewal-weighted one, because what this routing is for is the free tier;
// set a provider's own routing (Smart, in order, weekly pace) if the paid
// seats should be spent smartly among themselves — a provider or group in
// this group keeps its own routing either way.
//
// A seat whose model costs nothing nowhere is not free: it goes with the
// paid ones, so a routing set on a pool without free models behaves as In
// order.
var FreeFirst = Strategy{ByFree: true, Order: func(_ string, seats []Seat) []int {
	free, paid := make([]int, 0, len(seats)), make([]int, 0, len(seats))
	for i, s := range seats {
		if s.Free {
			free = append(free, i)
		} else {
			paid = append(paid, i)
		}
	}
	return append(free, paid...)
}}

// ExpireHorizon is how soon credits running out put a seat ahead: two days,
// wide enough to catch a balance lapsing over a night the user is not at the
// computer, and WorkBuddy's soonest package.
const ExpireHorizon = 48 * time.Hour

// The hours are this user's own (2026-10-28): at work 09:00–12:00 and
// 13:30–18:00, at the computer most evenings until 23:00 or so. The boundary is
// the end of the lunch break, and it is the only one: the night, the morning
// and the lunch break all end there. The morning counts too even though the
// user is back at 09:00 — a balance lapsing at 09:10 or 10:00 is not spent just
// because someone is at the keyboard, 91 credits being some 1,500 requests at
// 0.06 each. Spending it the evening before is what makes it spent at all.
//
// They are constants to be edited, not settings: the deadline only moves a
// conversation off the account whose cache is warm, and that costs a cache
// read of the whole conversation, so it is worth getting right.
var (
	awayLunch   = 13 // the lunch break ends, and the user is back
	awayLunchAt = 30
)

// UrgentLeast is how much of a balance has to be about to go for a conversation
// to be moved off the account whose cache is warm: below it, the cache read
// costs more than the balance is worth. In the unit the reading names (credits,
// 积分), so a vendor whose points are worth another's hundredth wants its own
// line in the map.
var (
	urgentLeast    = 50.0
	urgentLeastFor = map[string]float64{}
)

// UrgentLeastOf is UrgentLeast for one provider, its own line first.
func UrgentLeastOf(provider string) float64 {
	if v, ok := urgentLeastFor[provider]; ok {
		return v
	}
	return urgentLeast
}

// UrgentUntil is the end of the next window this user is away from the
// computer: the deadline within which what a seat still has is lost for good,
// whatever the conversation's cache would rather do. A balance lapsing while
// nobody is here is spent by nobody, so it is worth a cache to spend it now.
func UrgentUntil(now time.Time) time.Time {
	y, m, d := now.Date()
	at := func(day int) time.Time {
		return time.Date(y, m, day, awayLunch, awayLunchAt, 0, 0, now.Location())
	}
	if now.Before(at(d)) {
		return at(d)
	}
	return at(d + 1)
}

// ExpireFirst spends what is about to be lost before anything else, then
// what tells no day of its own, then what costs nothing, then the rest by
// how soon their credits run out:
//
//   - the seats whose credits run out within ExpireHorizon, the soonest
//     first — all of them before any other seat, so a package about to
//     lapse is spent on the day's requests;
//   - then the check-in credits (tierTwo: AutoClaw, MiniMax, Qoder, Kimi
//     Work) — the seats of those vendors whose model costs something; a
//     balance with no day of its own may be gone tomorrow, so it goes
//     before what merely costs nothing. A free model one of them lists is
//     not a balance to spend and stays with the free seats;
//   - then the models its vendor serves at no cost, in their listed order;
//   - then the rest, the soonest expiry first, one that never expires last.
//
// A seat at 98% or more of its allowance waits behind all of them, as in
// Smart: one about to be poured out is not worth pouring into.
//
// The order is worked out for each request, so a seat that comes to lapse
// within the horizon takes its place as it does, and one that rested after
// a rate limit answers again the moment its rest is over (resting seats
// then standing among the others, by this same order).
var ExpireFirst = Strategy{ByFree: true, Urgent: true, Order: func(_ string, seats []Seat) []int {
	now := time.Now()
	horizon := now.Add(ExpireHorizon)
	// due is the soonest renewal the seat's windows name, zero when none
	// does: credits expiring apart from a reset come in it as a renewal
	// (Allowance.Renewal), which is how a package about to lapse is seen.
	due := func(s Seat) time.Time {
		var out time.Time
		for _, t := range s.Soon {
			if t.IsZero() {
				continue
			}
			if out.IsZero() || t.Before(out) {
				out = t
			}
		}
		return out
	}
	// lapsing: it loses what it has left within the horizon, and still has it
	// to lose — and the model asked for is one that spends it. A model its
	// vendor serves at no cost spends nothing, so a balance expiring under
	// it loses nothing by waiting: ranked as lapsing, a free model took the
	// front from paid credits that really were about to go (商汤小浣熊's free
	// sn-deepseek-v4-1-flash, whose 300 credits lapse at 23:59, ahead of
	// WorkBuddy's 91 at 09:41 — 2026-10-28). A seat whose own traffic has
	// been shown to spend another package is the same case one step further:
	// nothing of its is lost by waiting (Seat.Futile). A day already past is
	// a reading gone old, not a balance about to go.
	lapsing := func(s Seat, at time.Time) bool {
		return !s.Free && !s.Futile && !at.IsZero() && at.After(now) && !at.After(horizon)
	}
	idx := identity(len(seats))
	sort.SliceStable(idx, func(a, b int) bool {
		sa, sb := seats[idx[a]], seats[idx[b]]
		// all but used up goes last, whatever it would be otherwise
		if xa, xb := tier(sa.Used) == 2, tier(sb.Used) == 2; xa != xb {
			return !xa
		}
		da, db := due(sa), due(sb)
		// 1. what lapses within the horizon, the soonest first
		if la, lb := lapsing(sa, da), lapsing(sb, db); la != lb {
			return la
		} else if la && !da.Equal(db) {
			return da.Before(db)
		}
		// 2. the check-in credits (tierTwo), in their listed order: among
		// themselves they stand as the pool names them
		if ua, ub := unmet(sa), unmet(sb); ua != ub {
			return ua
		} else if ua {
			return false
		}
		// 3. what costs nothing, in its listed order
		if sa.Free != sb.Free {
			return sa.Free
		}
		// 4. the rest, the soonest expiry first, one with none last
		if !da.Equal(db) {
			switch {
			case da.IsZero():
				return false
			case db.IsZero():
				return true
			}
			return da.Before(db)
		}
		return false
	})
	return idx
}}

// Builtin are the routings besides Smart and their strategies, as
// plugins/rotation registers them.
var Builtin = map[string]Strategy{
	provider.Ordered:     Ordered,
	provider.Rotate:      Rotate,
	provider.LeastUsed:   LeastUsed,
	provider.Pace:        Pace,
	provider.Weighted:    Weighted,
	provider.FreeFirst:   FreeFirst,
	provider.ExpireFirst: ExpireFirst,
}
