// Package conformance hosts the versioned cross-entry cron conformance corpus
// ("cron/conformance/fixture"). A single fixture set drives every public
// entry point of github.com/adhocore/gronx — Gronx.IsDue (Check), IsValid,
// NextTickAfter, PrevTickBefore and Gronx.BatchDue — with one shared set of
// expressions, timezones, start instants and expected relations.
//
// Design rules:
//   - Timezones come from the embedded tz database (time/tzdata); the corpus
//     never reads machine-local timezone data.
//   - Expectations are relational invariants first (ordering, due-ness,
//     cross-entry agreement). Exact wall-clock times are pinned only for
//     cases whose semantics are fixed by the cron standard itself.
//   - Every iteration is budgeted: gronx bounds its internal search loop,
//     and chained tick assertions are bounded by ChainBudget.
//   - The corpus is deterministic: replaying any case (or the whole corpus)
//     twice with the same input yields identical output.
package conformance

import (
	"fmt"
	"time"

	_ "time/tzdata" // embedded tz database: never resolve zones from the host machine
)

// CorpusVersion identifies this fixture set. It is embedded into every
// exported failure bundle so cached or replayed results always carry the
// version identity of the corpus that produced them.
const CorpusVersion = "cron/conformance/fixture/v1"

// ShuffleSeed is the fixed seed for deterministic corpus re-orderings used
// to prove that results do not depend on enumeration order.
const ShuffleSeed int64 = 1270

// ChainBudget bounds chained NextTick/PrevTick relation checks. The gronx
// search loop itself is internally budgeted; this caps the extra iterations
// the corpus performs on top of a successful tick.
const ChainBudget = 3

// Expectation describes what the public entries must produce for a Case.
// Exact instants (Next/Prev) are RFC3339 in the case zone and are only set
// for standard-clear cases; everything else is relational (NextBy bounds,
// Due/Valid flags, error expectations).
type Expectation struct {
	Valid   *bool  `json:"valid,omitempty"`
	Due     *bool  `json:"due,omitempty"`
	DueErr  bool   `json:"dueErr,omitempty"`
	Next    string `json:"next,omitempty"`
	NextBy  string `json:"nextBy,omitempty"`
	NextErr bool   `json:"nextErr,omitempty"`
	Prev    string `json:"prev,omitempty"`
	PrevErr bool   `json:"prevErr,omitempty"`
}

// Case is one corpus entry: a single expression evaluated through every
// public entry point from one fixed start instant in one embedded timezone.
// The ID prefix (before "/") names the contract category so test failure
// names identify the violated contract directly.
type Case struct {
	ID     string      `json:"id"`
	Expr   string      `json:"expr"`
	Zone   string      `json:"zone"`
	Start  string      `json:"start"`
	Equiv  []string    `json:"equiv,omitempty"`
	Expect Expectation `json:"expect"`
}

// Location resolves the case zone against the embedded tz database.
func (c Case) Location() (*time.Location, error) {
	if c.Zone == "UTC" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(c.Zone)
	if err != nil {
		return nil, fmt.Errorf("case %s: load embedded zone %q: %w", c.ID, c.Zone, err)
	}
	return loc, nil
}

// StartTime parses the fixed start instant of the case.
func (c Case) StartTime() (time.Time, error) {
	start, err := time.Parse(time.RFC3339, c.Start)
	if err != nil {
		return time.Time{}, fmt.Errorf("case %s: parse start %q: %w", c.ID, c.Start, err)
	}
	return start, nil
}

func bp(b bool) *bool { return &b }

// corpus is the fixed fixture set. It is never mutated at runtime; Corpus
// hands out copies. Categories: dst_gap, dst_fold, leap_day, month_end,
// sunday_alias, range_step, unreachable_date, invalid_field.
var corpus = []Case{
	{
		ID: "dst_gap/spring_forward_nonexistent_wallclock", Expr: "30 2 * * *",
		Zone: "America/New_York", Start: "2024-03-09T12:00:00-05:00",
		Expect: Expectation{Valid: bp(true), Due: bp(false), NextBy: "2024-03-12T00:00:00-04:00"},
	},
	{
		ID: "dst_fold/fall_back_ambiguous_wallclock", Expr: "30 1 * * *",
		Zone: "America/New_York", Start: "2024-11-02T12:00:00-04:00",
		Expect: Expectation{Valid: bp(true), Due: bp(false), NextBy: "2024-11-03T03:00:00-05:00"},
	},
	{
		ID: "leap_day/feb29_forward_and_backward", Expr: "0 0 29 2 *",
		Zone: "UTC", Start: "2023-03-01T00:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-02-29T00:00:00Z", Prev: "2020-02-29T00:00:00Z"},
	},
	{
		ID: "month_end/day31_skips_short_months", Expr: "0 0 31 * *",
		Zone: "UTC", Start: "2024-04-01T00:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-05-31T00:00:00Z", Prev: "2024-03-31T00:00:00Z"},
	},
	{
		ID: "month_end/L_last_day_of_leap_february", Expr: "0 0 L * *",
		Zone: "UTC", Start: "2024-02-15T00:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-02-29T00:00:00Z", Prev: "2024-01-31T00:00:00Z"},
	},
	{
		ID: "sunday_alias/due_on_sunday", Expr: "0 0 * * 7",
		Zone: "UTC", Start: "2024-01-07T00:00:00Z",
		Equiv:  []string{"0 0 * * 0", "0 0 * * SUN"},
		Expect: Expectation{Valid: bp(true), Due: bp(true)},
	},
	{
		ID: "sunday_alias/next_sunday_from_midweek", Expr: "0 0 * * 0",
		Zone: "UTC", Start: "2024-01-03T12:00:00Z",
		Equiv:  []string{"0 0 * * 7", "0 0 * * SUN"},
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-01-07T00:00:00Z"},
	},
	{
		ID: "range_step/minute_step_within_range", Expr: "0-30/10 * * * *",
		Zone: "UTC", Start: "2024-01-01T00:05:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-01-01T00:10:00Z", Prev: "2024-01-01T00:00:00Z"},
	},
	{
		ID: "range_step/weekday_and_hour_ranges", Expr: "*/15 9-17 * * 1-5",
		Zone: "UTC", Start: "2024-01-06T18:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), Next: "2024-01-08T09:00:00Z"},
	},
	{
		ID: "unreachable_date/february_30", Expr: "0 0 30 2 *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), NextErr: true, PrevErr: true},
	},
	{
		ID: "unreachable_date/april_31", Expr: "0 0 31 4 *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(true), Due: bp(false), NextErr: true, PrevErr: true},
	},
	{
		ID: "invalid_field/minute_out_of_bounds", Expr: "0 61 * * *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(false), DueErr: true, NextErr: true, PrevErr: true},
	},
	{
		ID: "invalid_field/monthday_out_of_bounds", Expr: "0 0 32 * *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(false), DueErr: true, NextErr: true, PrevErr: true},
	},
	{
		ID: "invalid_field/zero_step", Expr: "*/0 * * * *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(false), DueErr: true, NextErr: true, PrevErr: true},
	},
	{
		ID: "invalid_field/too_many_segments", Expr: "* * * * * * * *",
		Zone: "UTC", Start: "2024-01-01T00:00:00Z",
		Expect: Expectation{Valid: bp(false), DueErr: true, NextErr: true, PrevErr: true},
	},
}

// Corpus returns a copy of the versioned fixture set in canonical order.
func Corpus() []Case {
	out := make([]Case, len(corpus))
	copy(out, corpus)
	return out
}
