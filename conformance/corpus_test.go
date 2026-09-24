package conformance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/adhocore/gronx"
	"github.com/adhocore/gronx/conformance"
)

func boolp(b bool) *bool { return &b }

// TestCorpusContract runs the whole versioned corpus through Check (IsDue),
// IsValid, NextTick, PrevTick and the batch entry. Subtest names carry the
// contract category (dst_gap/..., invalid_field/...) so a failure names the
// broken contract directly.
func TestCorpusContract(t *testing.T) {
	cases := conformance.Corpus()
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			_, violations := conformance.RunCase(c)
			for _, v := range violations {
				t.Errorf("[%s] %s: want %s, got %s", v.Entry, v.Context, v.Want, v.Got)
			}
		})
	}

	t.Run("corpus_report", func(t *testing.T) {
		rep := conformance.Run(cases)
		if rep.Version != conformance.CorpusVersion {
			t.Errorf("report version: want %s, got %s", conformance.CorpusVersion, rep.Version)
		}
		if !rep.OK() {
			v := rep.FirstViolation()
			t.Fatalf("first violation: case=%s entry=%s ctx=%s want=%s got=%s (total %d)",
				v.CaseID, v.Entry, v.Context, v.Want, v.Got, len(rep.Violations))
		}
	})
}

// TestCorpusDeterministicReplay proves the fixed-seed corpus replays with
// identical output twice, as a whole and case by case.
func TestCorpusDeterministicReplay(t *testing.T) {
	cases := conformance.Corpus()
	first, second := conformance.Run(cases), conformance.Run(cases)
	if first.Digest() != second.Digest() {
		t.Errorf("two full runs differ:\n%s\n%s", first.Digest(), second.Digest())
	}

	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			o1, v1 := conformance.RunCase(c)
			o2, v2 := conformance.RunCase(c)
			if !reflect.DeepEqual(o1, o2) || !reflect.DeepEqual(v1, v2) {
				t.Errorf("single-case replay not deterministic: %+v/%+v vs %+v/%+v", o1, v1, o2, v2)
			}
		})
	}
}

// TestCorpusOrderStability changes only result-irrelevant enumeration and
// scheduling conditions (case order, batch order, duplicates, host TZ) and
// requires identical per-case outcomes.
func TestCorpusOrderStability(t *testing.T) {
	cases := conformance.Corpus()
	base := conformance.Run(cases)
	byID := map[string]conformance.Outcome{}
	for _, o := range base.Outcomes {
		byID[o.ID] = o
	}

	reversed := make([]conformance.Case, len(cases))
	for i, c := range cases {
		reversed[len(cases)-1-i] = c
	}
	for name, reordered := range map[string][]conformance.Case{
		"reversed": reversed,
		"shuffled": conformance.Shuffled(cases, conformance.ShuffleSeed),
	} {
		t.Run(name, func(t *testing.T) {
			rep := conformance.Run(reordered)
			if len(rep.Outcomes) != len(byID) {
				t.Fatalf("outcome count: want %d, got %d", len(byID), len(rep.Outcomes))
			}
			for _, o := range rep.Outcomes {
				if !reflect.DeepEqual(byID[o.ID], o) {
					t.Errorf("case %s outcome changed under %s order: %+v vs %+v", o.ID, name, byID[o.ID], o)
				}
			}
		})
	}

	t.Run("batch_order_and_duplicates", func(t *testing.T) {
		ref := time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC)
		var exprs []string
		for _, c := range cases {
			exprs = append(exprs, c.Expr)
		}
		g := gronx.New()
		forward := g.BatchDue(exprs, ref)

		var mixed []string
		for i := len(exprs) - 1; i >= 0; i-- {
			mixed = append(mixed, exprs[i], exprs[i]) // reversed + duplicated
		}
		back := g.BatchDue(mixed, ref)

		got := map[string]gronx.Expr{}
		for _, e := range back {
			got[e.Expr] = e
		}
		for _, want := range forward {
			if got[want.Expr].Due != want.Due || (got[want.Expr].Err != nil) != (want.Err != nil) {
				t.Errorf("batch result for %q depends on order/duplicates: %+v vs %+v", want.Expr, want, got[want.Expr])
			}
		}
	})

	t.Run("host_tz_irrelevant", func(t *testing.T) {
		before := conformance.Run(cases).Digest()
		t.Setenv("TZ", "Pacific/Kiritimati")
		if after := conformance.Run(cases).Digest(); before != after {
			t.Errorf("corpus digest depends on host TZ: %s vs %s", before, after)
		}
	})
}

// flakyChecker injects one recoverable failure into the real CheckDue chain.
type flakyChecker struct {
	inner  gronx.Checker
	failAt int
	calls  int
}

var errInjected = errors.New("injected recoverable failure")

func (f *flakyChecker) GetRef() time.Time  { return f.inner.GetRef() }
func (f *flakyChecker) SetRef(t time.Time) { f.inner.SetRef(t) }
func (f *flakyChecker) CheckDue(seg string, pos int) (bool, error) {
	f.calls++
	if f.calls == f.failAt {
		return false, errInjected
	}
	return f.inner.CheckDue(seg, pos)
}

// TestCorpusRecoverableFailure injects a recoverable failure mid-run, then
// proves: the failure is reported at the injected case only, the failed run
// is itself deterministic, sibling outcomes carry no cache/count/temp state
// from the failed call, and a retry with identical input matches the clean
// baseline exactly.
func TestCorpusRecoverableFailure(t *testing.T) {
	cases := conformance.Corpus()
	baseline := conformance.Run(cases)
	if !baseline.OK() {
		t.Fatalf("baseline not clean: %+v", baseline.FirstViolation())
	}

	mid := len(cases) / 2
	// Each injection uses a fresh flaky checker so the failure itself carries
	// no count state across runs and the failed run stays deterministic.
	var injected *flakyChecker
	runner := conformance.Runner{NewGronx: func(c conformance.Case) *gronx.Gronx {
		g := gronx.New()
		if c.ID == cases[mid].ID {
			injected = &flakyChecker{inner: &gronx.SegmentChecker{}, failAt: 1}
			g.C = injected
		}
		return g
	}}

	failed := runner.Run(cases)
	failedAgain := runner.Run(cases)

	t.Run("failure_is_localized_and_deterministic", func(t *testing.T) {
		if failed.OK() {
			t.Fatal("injected failure did not surface")
		}
		for _, v := range failed.Violations {
			if v.CaseID != cases[mid].ID {
				t.Errorf("violation leaked outside injected case: %+v", v)
			}
		}
		if failed.Digest() != failedAgain.Digest() {
			t.Errorf("failed run not deterministic: %s vs %s", failed.Digest(), failedAgain.Digest())
		}
	})

	t.Run("no_state_leaks_into_siblings", func(t *testing.T) {
		if injected == nil || injected.calls == 0 {
			t.Fatal("injected checker was never exercised")
		}
		for i, o := range failed.Outcomes {
			if i == mid {
				continue
			}
			if !reflect.DeepEqual(o, baseline.Outcomes[i]) {
				t.Errorf("case %s outcome polluted by failed call: %+v vs %+v", o.ID, o, baseline.Outcomes[i])
			}
		}
	})

	t.Run("retry_after_recovery_matches_baseline", func(t *testing.T) {
		recovered := conformance.Run(cases)
		if recovered.Digest() != baseline.Digest() {
			t.Errorf("recovered run differs from baseline: %s vs %s", recovered.Digest(), baseline.Digest())
		}
		if !recovered.OK() {
			t.Errorf("recovered run still failing: %+v", recovered.FirstViolation())
		}
	})
}

// TestCorpusFailureBundleExport proves a violation can be exported as a
// minimal, versioned, deterministic bundle and replayed in isolation.
func TestCorpusFailureBundleExport(t *testing.T) {
	c := conformance.Corpus()[0] // dst_gap case, valid and due=false at start
	broken := c
	broken.Expect.Due = boolp(true) // tampered expectation -> real violation from real entries

	_, violations := conformance.RunCase(broken)
	if len(violations) == 0 {
		t.Fatal("tampered expectation produced no violation")
	}
	first := violations[0]
	bundle := conformance.NewBundle(broken, first)

	t.Run("bundle_is_minimal_and_versioned", func(t *testing.T) {
		if bundle.Version != conformance.CorpusVersion {
			t.Errorf("bundle version: want %s, got %s", conformance.CorpusVersion, bundle.Version)
		}
		if bundle.Case.ID != c.ID || bundle.Entry == "" || bundle.Want == "" || bundle.Got == "" {
			t.Errorf("bundle missing context: %+v", bundle)
		}
	})

	t.Run("export_is_deterministic", func(t *testing.T) {
		var b1, b2 bytes.Buffer
		if err := bundle.Export(&b1); err != nil {
			t.Fatal(err)
		}
		if err := bundle.Export(&b2); err != nil {
			t.Fatal(err)
		}
		if b1.String() != b2.String() {
			t.Errorf("two exports differ:\n%s\n%s", b1.String(), b2.String())
		}
	})

	t.Run("bundle_roundtrip_and_replay", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "failure-bundle.json")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := bundle.Export(f); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var loaded conformance.Bundle
		if err := json.Unmarshal(payload, &loaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded, bundle) {
			t.Errorf("bundle roundtrip mismatch: %+v vs %+v", loaded, bundle)
		}

		_, replayed := loaded.Replay()
		if len(replayed) == 0 {
			t.Fatal("replayed bundle produced no violation")
		}
		if replayed[0].Entry != first.Entry || replayed[0].Want != first.Want || replayed[0].Got != first.Got {
			t.Errorf("replayed violation differs: %+v vs %+v", replayed[0], first)
		}
		// Replaying twice from the same bundle is stable.
		_, replayedAgain := loaded.Replay()
		if !reflect.DeepEqual(replayed, replayedAgain) {
			t.Errorf("bundle replay not deterministic: %+v vs %+v", replayed, replayedAgain)
		}
	})
}

// TestCorpusEmbeddedTimezone proves the DST corpus cases run on embedded,
// controllable timezone rules (not machine-local data): the exact UTC
// instants of the 2024 America/New_York transitions are known to the corpus.
func TestCorpusEmbeddedTimezone(t *testing.T) {
	var gapCase, foldCase *conformance.Case
	for i, c := range conformance.Corpus() {
		switch c.ID {
		case "dst_gap/spring_forward_nonexistent_wallclock":
			gapCase = &conformance.Corpus()[i]
		case "dst_fold/fall_back_ambiguous_wallclock":
			foldCase = &conformance.Corpus()[i]
		}
	}
	if gapCase == nil || foldCase == nil {
		t.Fatal("DST cases missing from corpus")
	}

	loc, err := gapCase.Location()
	if err != nil {
		t.Fatal(err)
	}
	offsetAt := func(instant string) int {
		ts, _ := time.Parse(time.RFC3339, instant)
		_, off := ts.In(loc).Zone()
		return off
	}

	// Spring forward 2024-03-10 07:00 UTC: -5h before, -4h after.
	if off := offsetAt("2024-03-10T06:59:59Z"); off != -5*3600 {
		t.Errorf("pre-gap offset: want -5h, got %d", off)
	}
	if off := offsetAt("2024-03-10T07:00:00Z"); off != -4*3600 {
		t.Errorf("post-gap offset: want -4h, got %d", off)
	}
	// Fall back 2024-11-03 06:00 UTC: -4h before, -5h after.
	if off := offsetAt("2024-11-03T05:59:59Z"); off != -4*3600 {
		t.Errorf("pre-fold offset: want -4h, got %d", off)
	}
	if off := offsetAt("2024-11-03T06:00:00Z"); off != -5*3600 {
		t.Errorf("post-fold offset: want -5h, got %d", off)
	}
}
