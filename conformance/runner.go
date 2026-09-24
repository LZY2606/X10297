package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/adhocore/gronx"
)

// Outcome records the caller-visible results of running one case through every
// public entry point. It is the replayable, comparable unit of the corpus.
type Outcome struct {
	ID       string `json:"id"`
	Valid    bool   `json:"valid"`
	Due      bool   `json:"due"`
	DueErr   string `json:"dueErr,omitempty"`
	BatchDue bool   `json:"batchDue"`
	BatchErr string `json:"batchErr,omitempty"`
	Next     string `json:"next,omitempty"`
	NextErr  string `json:"nextErr,omitempty"`
	Prev     string `json:"prev,omitempty"`
	PrevErr  string `json:"prevErr,omitempty"`
}

// Violation pinpoints the first (or any) broken expectation: which case,
// which public entry, the context, and got/want. It never degrades to a
// bare boolean.
type Violation struct {
	CaseID  string `json:"caseId"`
	Entry   string `json:"entry"`
	Context string `json:"context"`
	Want    string `json:"want"`
	Got     string `json:"got"`
}

// Report is the result of running a set of cases.
type Report struct {
	Version    string      `json:"version"`
	Outcomes   []Outcome   `json:"outcomes"`
	Violations []Violation `json:"violations,omitempty"`
}

// OK reports whether the run produced no violations.
func (r Report) OK() bool { return len(r.Violations) == 0 }

// FirstViolation returns the first violation in execution order, if any.
func (r Report) FirstViolation() *Violation {
	if len(r.Violations) == 0 {
		return nil
	}
	return &r.Violations[0]
}

// Digest is a stable fingerprint of the caller-visible outcomes and
// violations. Two runs with identical inputs must share a digest.
func (r Report) Digest() string {
	payload, _ := json.Marshal(r)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Runner executes corpus cases through the public gronx entry points.
type Runner struct {
	// NewGronx optionally customizes how the Gronx used for IsDue/BatchDue is
	// built for a case (e.g. recoverable fault injection in tests). Nil means
	// gronx.New. NextTickAfter/PrevTickBefore are package-level entries and
	// always construct their own Gronx internally.
	NewGronx func(c Case) *gronx.Gronx
}

func (r Runner) gronxFor(c Case) *gronx.Gronx {
	if r.NewGronx != nil {
		return r.NewGronx(c)
	}
	return gronx.New()
}

// Run executes every case in the given order and collects the report.
func (r Runner) Run(cases []Case) Report {
	rep := Report{Version: CorpusVersion}
	for _, c := range cases {
		outcome, violations := r.RunCase(c)
		rep.Outcomes = append(rep.Outcomes, outcome)
		rep.Violations = append(rep.Violations, violations...)
	}
	return rep
}

// Run executes the full corpus in canonical order with a default Runner.
func Run(cases []Case) Report { return Runner{}.Run(cases) }

// RunCase replays a single case with a default Runner.
func RunCase(c Case) (Outcome, []Violation) { return Runner{}.RunCase(c) }

// RunCase executes one case through Check (IsDue), IsValid, NextTickAfter,
// PrevTickBefore and the batch entry, then verifies the case expectations
// and the cross-entry relational invariants.
func (r Runner) RunCase(c Case) (Outcome, []Violation) {
	out := Outcome{ID: c.ID}
	var violations []Violation
	fail := func(entry, want, got string) {
		violations = append(violations, Violation{
			CaseID:  c.ID,
			Entry:   entry,
			Context: fmt.Sprintf("expr=%q zone=%s start=%s", c.Expr, c.Zone, c.Start),
			Want:    want,
			Got:     got,
		})
	}

	loc, err := c.Location()
	if err != nil {
		fail("LoadLocation", "embedded zone resolves", err.Error())
		return out, violations
	}
	start, err := c.StartTime()
	if err != nil {
		fail("ParseStart", "RFC3339 start parses", err.Error())
		return out, violations
	}

	g := r.gronxFor(c)
	due, dueErr := g.IsDue(c.Expr, start)
	out.Due, out.DueErr = due, errString(dueErr)
	out.Valid = gronx.IsValid(c.Expr)

	batch := g.BatchDue([]string{c.Expr, c.Expr}, start)
	out.BatchDue, out.BatchErr = batch[0].Due, errString(batch[0].Err)

	next, nextErr := gronx.NextTickAfter(c.Expr, start, false)
	if nextErr == nil {
		out.Next = next.In(loc).Format(time.RFC3339)
	}
	out.NextErr = errString(nextErr)

	prev, prevErr := gronx.PrevTickBefore(c.Expr, start, false)
	if prevErr == nil {
		out.Prev = prev.In(loc).Format(time.RFC3339)
	}
	out.PrevErr = errString(prevErr)

	exp := c.Expect
	if exp.Valid != nil && out.Valid != *exp.Valid {
		fail("IsValid", fmt.Sprintf("valid=%v", *exp.Valid), fmt.Sprintf("valid=%v", out.Valid))
	}
	if (dueErr != nil) != exp.DueErr {
		fail("IsDue", fmt.Sprintf("err=%v", exp.DueErr), fmt.Sprintf("err=%v", dueErr))
	}
	if exp.Due != nil && dueErr == nil && due != *exp.Due {
		fail("IsDue", fmt.Sprintf("due=%v", *exp.Due), fmt.Sprintf("due=%v", due))
	}
	// Relational: an expression that errors on check must not be valid.
	if dueErr != nil && out.Valid {
		fail("IsValid/IsDue", "invalid when check errors", "valid=true")
	}
	// Relational: the batch entry agrees with the single check entry.
	if batch[0].Due != due || (batch[0].Err != nil) != (dueErr != nil) {
		fail("BatchDue", fmt.Sprintf("due=%v err=%v", due, dueErr != nil),
			fmt.Sprintf("due=%v err=%v", batch[0].Due, batch[0].Err))
	}
	// Relational: duplicates inside one batch (cache path) agree.
	if batch[1].Due != batch[0].Due || (batch[1].Err != nil) != (batch[0].Err != nil) {
		fail("BatchDue/duplicate", "duplicate expr agrees", fmt.Sprintf("%+v", batch[1]))
	}

	if (nextErr != nil) != exp.NextErr {
		fail("NextTick", fmt.Sprintf("err=%v", exp.NextErr), fmt.Sprintf("err=%v", nextErr))
	}
	if nextErr == nil {
		if !next.After(start) {
			fail("NextTick", "next > start", out.Next)
		}
		if ok, _ := r.gronxFor(c).IsDue(c.Expr, next); !ok {
			fail("NextTick/IsDue", "next is due", out.Next)
		}
		if exp.Next != "" && out.Next != exp.Next {
			fail("NextTick", exp.Next, out.Next)
		}
		if exp.NextBy != "" {
			if bound, berr := time.Parse(time.RFC3339, exp.NextBy); berr == nil && !next.Before(bound) {
				fail("NextTick", "next < "+exp.NextBy, out.Next)
			}
		}
		// Budgeted chain: consecutive ticks strictly increase and stay due.
		cursor := next
		for i := 0; i < ChainBudget; i++ {
			follow, ferr := gronx.NextTickAfter(c.Expr, cursor, false)
			if ferr != nil || !follow.After(cursor) {
				fail("NextTick/chain", "strictly increasing tick", fmt.Sprintf("%s err=%v", follow, ferr))
				break
			}
			if ok, _ := r.gronxFor(c).IsDue(c.Expr, follow); !ok {
				fail("NextTick/chain", "chained tick is due", follow.In(loc).Format(time.RFC3339))
				break
			}
			cursor = follow
		}
	}

	if (prevErr != nil) != exp.PrevErr {
		fail("PrevTick", fmt.Sprintf("err=%v", exp.PrevErr), fmt.Sprintf("err=%v", prevErr))
	}
	if prevErr == nil {
		if !prev.Before(start) {
			fail("PrevTick", "prev < start", out.Prev)
		}
		if ok, _ := r.gronxFor(c).IsDue(c.Expr, prev); !ok {
			fail("PrevTick/IsDue", "prev is due", out.Prev)
		}
		if exp.Prev != "" && out.Prev != exp.Prev {
			fail("PrevTick", exp.Prev, out.Prev)
		}
	}
	if nextErr == nil && prevErr == nil && !prev.Before(next) {
		fail("PrevTick/NextTick", "prev < next", fmt.Sprintf("prev=%s next=%s", out.Prev, out.Next))
	}

	// Relational: inclusive tick returns start iff start itself is due.
	if incl, inclErr := gronx.NextTickAfter(c.Expr, start, true); inclErr == nil && dueErr == nil {
		if due && !incl.Equal(start) {
			fail("NextTick/inclusive", "start when start is due", incl.In(loc).Format(time.RFC3339))
		}
		if !due && nextErr == nil && !incl.Equal(next) {
			fail("NextTick/inclusive", "same as exclusive when start not due", incl.In(loc).Format(time.RFC3339))
		}
	}

	// Relational: alias expressions agree on every entry.
	for _, alias := range c.Equiv {
		aDue, aErr := r.gronxFor(c).IsDue(alias, start)
		if aDue != due || (aErr != nil) != (dueErr != nil) {
			fail("Equiv/IsDue", fmt.Sprintf("%q agrees", alias), fmt.Sprintf("due=%v err=%v", aDue, aErr))
		}
		if gronx.IsValid(alias) != out.Valid {
			fail("Equiv/IsValid", fmt.Sprintf("%q agrees", alias), fmt.Sprintf("valid=%v", gronx.IsValid(alias)))
		}
		if aNext, aNextErr := gronx.NextTickAfter(alias, start, false); (aNextErr != nil) != (nextErr != nil) || (aNextErr == nil && !aNext.Equal(next)) {
			fail("Equiv/NextTick", fmt.Sprintf("%q agrees", alias), aNext.In(loc).Format(time.RFC3339))
		}
		if aPrev, aPrevErr := gronx.PrevTickBefore(alias, start, false); (aPrevErr != nil) != (prevErr != nil) || (aPrevErr == nil && !aPrev.Equal(prev)) {
			fail("Equiv/PrevTick", fmt.Sprintf("%q agrees", alias), aPrev.In(loc).Format(time.RFC3339))
		}
	}

	return out, violations
}

// Shuffled returns a deterministic re-ordering of cases for the fixed seed.
// It changes only enumeration order, never any input, so per-case outcomes
// must be identical to canonical order.
func Shuffled(cases []Case, seed int64) []Case {
	out := make([]Case, len(cases))
	copy(out, cases)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// Bundle is the minimal replayable failure package: one versioned case plus
// one violation, exportable as deterministic JSON.
type Bundle struct {
	Version string `json:"version"`
	Case    Case   `json:"case"`
	Entry   string `json:"entry"`
	Context string `json:"context"`
	Want    string `json:"want"`
	Got     string `json:"got"`
}

// NewBundle packs a case and its violation into a minimal failure bundle.
func NewBundle(c Case, v Violation) Bundle {
	return Bundle{
		Version: CorpusVersion,
		Case:    c,
		Entry:   v.Entry,
		Context: v.Context,
		Want:    v.Want,
		Got:     v.Got,
	}
}

// Export writes the bundle as deterministic (indented, key-ordered) JSON.
func (b Bundle) Export(w io.Writer) error {
	payload, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(payload, '\n'))
	return err
}

// Replay re-executes the bundled case through the public entries and returns
// its fresh outcome and violations, so a failure can be reproduced in
// isolation from the exported package alone.
func (b Bundle) Replay() (Outcome, []Violation) {
	return RunCase(b.Case)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
