package gronx

// This file is the "cron/conformance/fixture" regression material for gronx:
// a single versioned corpus of minimal inputs shared by Check (IsDue),
// IsValid, NextTick, PrevTick and the BatchDue entry. It is consumed only by
// the conformance tests in corpus_test.go, and every case is driven through
// the package's real public entries - no internal result object is forged.

import (
	"time"

	// Embed the IANA tz database into the test binary so DST gap/fold cases
	// never depend on (or even read) the machine's local zoneinfo database.
	_ "time/tzdata"
)

// corpusVersion is the identity of this fixture and of every cache/replay
// derived from it: bump it when the corpus semantics change.
const corpusVersion = "gronx.cron/conformance@v1"

// corpusSeed fixes all enumeration (shuffle/shrink) randomness, so a shrunk
// failure bundle replays standalone and two consecutive runs agree exactly.
const corpusSeed int64 = 20260919

// corpusChainBudget bounds the bounded-iteration invariant check. All
// iteration the corpus performs has this explicit budget.
const corpusChainBudget = 4

const corpusTimeLayout = "2006-01-02 15:04:05"

func bptr(b bool) *bool { return &b }

// conformanceCorpus returns the frozen corpus. Expectations prefer relational
// invariants (due/order/batch agreement); exact instants are pinned only for
// the standard-clear UTC cases. DST gap/fold cases are relation-only.
func conformanceCorpus() []corpusCase {
	dueRelations := []relation{
		relNextDue, relNextAfterStart, relPrevDue, relPrevBeforeStart,
		relBracket, relBatchAgrees, relChainDue,
	}
	invalidRelations := []relation{relBatchAgrees}

	return []corpusCase{
		// --- DST gap: 2024-03-10 02:30 never exists in America/New_York.
		// The exact landing time is implementation-defined; the corpus only
		// fixes relational semantics for it.
		{
			ID: "dst-gap-ny", Tags: []string{"dst-gap"},
			Expr: "0 30 2 10 3 *", Zone: "America/New_York", Start: "2024-03-09 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Relations: dueRelations,
		},
		// --- DST fold: 2024-11-03 01:30 happens twice (EDT then EST).
		{
			ID: "dst-fold-ny", Tags: []string{"dst-fold"},
			Expr: "0 30 1 3 11 *", Zone: "America/New_York", Start: "2024-11-02 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Relations: dueRelations,
		},
		// --- Leap day, exclusive start: skips 2023 (not a leap year).
		{
			ID: "leap-day-next", Tags: []string{"leap-day"},
			Expr: "0 0 29 2 *", Zone: "UTC", Start: "2023-06-01 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Next: "2024-02-29 00:00:00 +0000", Prev: "2020-02-29 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Leap day exactly at a due instant, inclusive reference.
		{
			ID: "leap-day-inclusive", Tags: []string{"leap-day"},
			Expr: "0 0 29 2 *", Zone: "UTC", Start: "2024-02-29 00:00:00", Inclusive: true,
			Valid: bptr(true), Due: bptr(true), DueErr: bptr(false),
			Next: "2024-02-29 00:00:00 +0000", Prev: "2024-02-29 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Month end L: February 2024 ends on the 29th (leap).
		{
			ID: "month-end-l-feb", Tags: []string{"month-end"},
			Expr: "0 0 L * *", Zone: "UTC", Start: "2024-02-01 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Next: "2024-02-29 00:00:00 +0000", Prev: "2024-01-31 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Month end L: 30-day month.
		{
			ID: "month-end-l-apr", Tags: []string{"month-end"},
			Expr: "0 0 L * *", Zone: "UTC", Start: "2024-04-15 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Next: "2024-04-30 00:00:00 +0000", Prev: "2024-03-31 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Sunday aliases 0 / 7 / SUN must agree, starting on a Sunday.
		{
			ID: "sunday-alias-zero", Tags: []string{"sunday-alias"},
			Expr: "0 0 * * 0", Zone: "UTC", Start: "2024-09-22 00:00:00",
			Valid: bptr(true), Due: bptr(true), DueErr: bptr(false),
			Next: "2024-09-29 00:00:00 +0000", Prev: "2024-09-15 00:00:00 +0000",
			Relations: dueRelations,
		},
		{
			ID: "sunday-alias-seven", Tags: []string{"sunday-alias"},
			Expr: "0 0 * * 7", Zone: "UTC", Start: "2024-09-22 00:00:00",
			Valid: bptr(true), Due: bptr(true), DueErr: bptr(false),
			Next: "2024-09-29 00:00:00 +0000", Prev: "2024-09-15 00:00:00 +0000",
			Relations: dueRelations,
		},
		{
			ID: "sunday-alias-name", Tags: []string{"sunday-alias"},
			Expr: "0 0 * * SUN", Zone: "UTC", Start: "2024-09-22 00:00:00",
			Valid: bptr(true), Due: bptr(true), DueErr: bptr(false),
			Next: "2024-09-29 00:00:00 +0000", Prev: "2024-09-15 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Range with step on weekday field.
		{
			ID: "range-step-weekday", Tags: []string{"range-step"},
			Expr: "0 9 * * 1-5/2", Zone: "UTC", Start: "2024-09-22 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Next: "2024-09-23 09:00:00 +0000", Prev: "2024-09-20 09:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Range with step on minute field.
		{
			ID: "range-step-minute", Tags: []string{"range-step"},
			Expr: "0-30/10 * * * *", Zone: "UTC", Start: "2024-09-22 00:05:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			Next: "2024-09-22 00:10:00 +0000", Prev: "2024-09-22 00:00:00 +0000",
			Relations: dueRelations,
		},
		// --- Unreachable date: day 30 never exists in February.
		{
			ID: "unreachable-feb30", Tags: []string{"unreachable"},
			Expr: "0 0 30 2 *", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			NextErr: bptr(true), NextErrContains: "tried so hard",
			PrevErr: bptr(true), PrevErrContains: "tried so hard",
			Relations: invalidRelations,
		},
		// --- Unreachable future year; backward direction still resolves.
		{
			ID: "unreachable-year-past", Tags: []string{"unreachable"},
			Expr: "0 0 1 1 * 2020", Zone: "UTC", Start: "2024-06-01 00:00:00",
			Valid: bptr(true), Due: bptr(false), DueErr: bptr(false),
			NextErr: bptr(true), NextErrContains: "unreachable year segment",
			Prev: "2020-01-01 00:00:00 +0000",
			Relations: []relation{relPrevDue, relPrevBeforeStart, relBatchAgrees},
		},
		// --- Illegal fields: rejection must fit the existing error layering
		// and be repeatable across entries.
		{
			ID: "illegal-minute", Tags: []string{"illegal"},
			Expr: "61 * * * *", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "out of bounds(0, 59)",
			PrevErr: bptr(true), PrevErrContains: "out of bounds(0, 59)",
			Relations: invalidRelations,
		},
		{
			ID: "illegal-weekday", Tags: []string{"illegal"},
			Expr: "* * * * 8", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "out of bounds(0, 7)",
			PrevErr: bptr(true), PrevErrContains: "out of bounds(0, 7)",
			Relations: invalidRelations,
		},
		{
			ID: "illegal-monthday-w", Tags: []string{"illegal"},
			Expr: "0 0 32W * *", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "out of bounds(1, 31)",
			PrevErr: bptr(true), PrevErrContains: "out of bounds(1, 31)",
			Relations: invalidRelations,
		},
		{
			ID: "illegal-weekday-nth", Tags: []string{"illegal"},
			Expr: "0 0 * * 5#6", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "nth: 1-5",
			PrevErr: bptr(true), PrevErrContains: "nth: 1-5",
			Relations: invalidRelations,
		},
		{
			ID: "illegal-arity", Tags: []string{"illegal"},
			Expr: "* * * *", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "5-7 segments",
			PrevErr: bptr(true), PrevErrContains: "5-7 segments",
			Relations: invalidRelations,
		},
		{
			ID: "illegal-step-zero", Tags: []string{"illegal"},
			Expr: "*/0 * * * *", Zone: "UTC", Start: "2024-01-01 00:00:00",
			Valid: bptr(false), Due: bptr(false), DueErr: bptr(true),
			NextErr: bptr(true), NextErrContains: "step can't be 0",
			PrevErr: bptr(true), PrevErrContains: "step can't be 0",
			Relations: invalidRelations,
		},
	}
}

// corpusStart parses a corpus start against an embedded-tzdata zone.
func corpusStart(zone, start string) time.Time {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		panic("corpus: embedded zone load failed: " + zone + ": " + err.Error())
	}
	tm, err := time.ParseInLocation(corpusTimeLayout, start, loc)
	if err != nil {
		panic("corpus: bad start " + start + ": " + err.Error())
	}
	return tm
}
