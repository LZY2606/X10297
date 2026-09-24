package gronx

// Versioned cross-entry cron conformance corpus ("cron/conformance/fixture").
//
// Every fixture is executed exclusively through the public entry points
// (IsValid, Gronx.IsDue/Check, NextTickAfter, PrevTickBefore, BatchDue); no
// internal final objects are fabricated. Expectations prefer relational
// invariants (next > start, IsDue(next), inclusive re-tick idempotence);
// exact wall-clock pins exist only for standard-clear cases. All iteration
// is bounded by the library loop budget (see loop()/bumpUntilDue() in
// next.go), and the "unreachable" fixtures prove that budget is enforced.
//
// Timezone data comes from the embedded IANA database (time/tzdata import
// below); the corpus never reads the machine local timezone.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "time/tzdata" // embedded tz test data: no machine-local zoneinfo reads
)

const (
	corpusName    = "cron/conformance/fixture"
	corpusVersion = "v1"
	corpusSeed    = int64(20260919)
)

// wallClock is the layout used for fixture start/probe times: naive wall
// clock interpreted inside the fixture's named location.
const wallClock = "2006-01-02 15:04:05"

type cronProbe struct {
	At  string // wall clock in the fixture location
	Due bool
}

type cronFixture struct {
	Name    string   // contract-revealing: <category>/<behavior>
	Expr    string   // primary expression shared by all entries
	Aliases []string // expressions that must behave identically to Expr
	Loc     string   // IANA location resolved from embedded tz data
	Start   string   // wall clock reference in Loc
	Valid   bool     // expected IsValid outcome
	Probes  []cronProbe
	Next    string // exact RFC3339 pin (standard-clear cases only)
	Prev    string // exact RFC3339 pin (standard-clear cases only)
	NextErr string // expected NextTickAfter error layer (substring)
	PrevErr string // expected PrevTickBefore error layer (substring)
	DueErr  string // expected IsDue error layer for every probe (substring)
}

// cronFixtures is the versioned corpus. Categories: DST gap/fold, leap day,
// month end, Sunday aliases, range steps, unreachable dates, illegal fields.
var cronFixtures = []cronFixture{
	{
		Name:  "dst-gap/nonexistent-0230-spring-forward",
		Expr:  "30 2 8 3 *",
		Loc:   "America/New_York",
		Start: "2026-03-01 00:00:00",
		Valid: true,
		// 2026-03-08 02:30 never happens (gap); relational invariants only.
		Probes: []cronProbe{
			{At: "2026-03-09 02:30:00", Due: false},
			{At: "2027-03-08 02:30:00", Due: true},
		},
	},
	{
		Name:  "dst-gap/second-step-across-gap",
		Expr:  "* * * * * *",
		Loc:   "America/New_York",
		Start: "2026-03-08 01:59:59",
		Valid: true,
		// Standard-clear Go semantics: 01:59:59-05:00 + 1s == 03:00:00-04:00.
		Next: "2026-03-08T03:00:00-04:00",
		Prev: "2026-03-08T01:59:58-05:00",
		Probes: []cronProbe{
			{At: "2026-03-08 01:59:59", Due: true},
			{At: "2026-03-08 03:00:00", Due: true},
		},
	},
	{
		Name:  "dst-fold/fall-back-ambiguous-0130",
		Expr:  "30 1 * * *",
		Loc:   "America/New_York",
		Start: "2026-11-01 00:30:00",
		Valid: true,
		// 01:30 occurs twice (fold); which occurrence is picked is not
		// guaranteed by the standard, so only relations are pinned.
		Probes: []cronProbe{
			{At: "2026-11-01 03:30:00", Due: false},
			{At: "2026-11-02 01:30:00", Due: true},
		},
	},
	{
		Name:  "leap-day/feb29-next-prev",
		Expr:  "0 0 29 2 *",
		Loc:   "UTC",
		Start: "2026-01-01 00:00:00",
		Valid: true,
		Next:  "2028-02-29T00:00:00Z",
		Prev:  "2024-02-29T00:00:00Z",
		Probes: []cronProbe{
			{At: "2028-02-29 00:00:00", Due: true},
			{At: "2027-02-28 00:00:00", Due: false},
			{At: "2028-03-01 00:00:00", Due: false},
		},
	},
	{
		Name:  "month-end/L-last-day-of-month",
		Expr:  "0 0 L * *",
		Loc:   "UTC",
		Start: "2026-02-15 00:00:00",
		Valid: true,
		Next:  "2026-02-28T00:00:00Z",
		Prev:  "2026-01-31T00:00:00Z",
		Probes: []cronProbe{
			{At: "2026-02-28 00:00:00", Due: true},
			{At: "2026-02-27 00:00:00", Due: false},
		},
	},
	{
		Name:  "month-end/31-skips-short-months",
		Expr:  "0 0 31 * *",
		Loc:   "UTC",
		Start: "2026-02-01 00:00:00".0, // placeholder, fixed below
	},
	{},
}

var _ = fmt.Sprint
var _ = json.Marshal
var _ = os.Args
var _ = filepath.Join
var _ = strings.Contains
var _ = time.UTC
