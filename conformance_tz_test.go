package gronx

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

// conformanceZoneName is the synthetic timezone used by the conformance corpus.
// Its DST rules are compiled into an embedded TZif blob by conformanceTZData,
// so corpus runs never read the machine's local timezone database.
const conformanceZoneName = "Conformance/USEastern"

// nthWeekdayOfMonth gives the nth weekday of a month at given UTC hour.
func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, nth, hour int) time.Time {
	first := time.Date(year, month, 1, hour, 0, 0, 0, time.UTC)
	day := first.AddDate(0, 0, (7+int(weekday)-int(first.Weekday()))%7)
	return day.AddDate(0, 0, 7*(nth-1))
}

// conformanceTZData builds a TZif v2 blob for a US-Eastern-like zone with
// controlled rules: spring forward to UTC-4 on the second Sunday of March at
// 07:00 UTC, fall back to UTC-5 on the first Sunday of November at 06:00 UTC,
// for the years 2020-2030. The output is fully deterministic.
func conformanceTZData() []byte {
	stdOff, dstOff := int32(-5*3600), int32(-4*3600)

	var trans []int64
	var kinds []byte
	trans = append(trans, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	kinds = append(kinds, 0)
	for year := 2020; year <= 2030; year++ {
		trans = append(trans, nthWeekdayOfMonth(year, time.March, time.Sunday, 2, 7).Unix())
		kinds = append(kinds, 1)
		trans = append(trans, nthWeekdayOfMonth(year, time.November, time.Sunday, 1, 6).Unix())
		kinds = append(kinds, 0)
	}

	buf := make([]byte, 0, 256)
	header := func(counts [6]uint32) {
		buf = append(buf, 'T', 'Z', 'i', 'f', '2')
		buf = append(buf, make([]byte, 15)...)
		var b [4]byte
		for _, c := range counts {
			binary.BigEndian.PutUint32(b[:], c)
			buf = append(buf, b[:]...)
		}
	}
	header([6]uint32{}) // empty v1 section, all data lives in the v2 section
	header([6]uint32{0, 0, 0, uint32(len(trans)), 2, 8})

	var b8 [8]byte
	for _, tr := range trans {
		binary.BigEndian.PutUint64(b8[:], uint64(tr))
		buf = append(buf, b8[:]...)
	}
	buf = append(buf, kinds...)

	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], uint32(stdOff))
	buf = append(buf, b4[:]...)
	buf = append(buf, 0, 0) // EST: not dst, abbr index 0
	binary.BigEndian.PutUint32(b4[:], uint32(dstOff))
	buf = append(buf, b4[:]...)
	buf = append(buf, 1, 4) // EDT: dst, abbr index 4
	buf = append(buf, []byte("EST\x00EDT\x00")...)
	buf = append(buf, '\n')
	buf = append(buf, []byte("EST5EDT,M3.2.0/2,M11.1.0/2")...)
	buf = append(buf, '\n')
	return buf
}

// conformanceZone resolves a corpus zone name to a location. Only UTC and the
// embedded synthetic zone are available; time.Local is never consulted.
func conformanceZone(name string) (*time.Location, error) {
	switch name {
	case "UTC":
		return time.UTC, nil
	case conformanceZoneName:
		return time.LoadLocationFromTZData(conformanceZoneName, conformanceTZData())
	}
	return nil, fmt.Errorf("unknown corpus zone: %s", name)
}

// TestCronConformanceZoneEmbedded pins the embedded zone rules so a broken or
// machine-dependent zone fails here first, before any corpus case runs.
func TestCronConformanceZoneEmbedded(t *testing.T) {
	loc, err := conformanceZone(conformanceZoneName)
	if err != nil {
		t.Fatalf("load embedded zone: %v", err)
	}
	if loc == time.Local {
		t.Fatal("corpus zone must not be the machine local timezone")
	}

	t.Run("dst gap", func(t *testing.T) {
		// 2026-03-08 02:00-03:00 local does not exist; 03:30 is UTC-4 (EDT).
		gap, _ := time.ParseInLocation(FullDateFormat, "2026-03-08 03:30:00", loc)
		name, off := gap.Zone()
		if name != "EDT" || off != -4*3600 {
			t.Errorf("expected EDT/-14400 after spring forward, got %s/%d", name, off)
		}
		before, _ := time.ParseInLocation(FullDateFormat, "2026-03-08 01:30:00", loc)
		if name, off := before.Zone(); name != "EST" || off != -5*3600 {
			t.Errorf("expected EST/-18000 before spring forward, got %s/%d", name, off)
		}
	})

	t.Run("dst fold", func(t *testing.T) {
		// 2026-11-01 01:30 local happens twice: first UTC-4, then UTC-5.
		first, _ := time.ParseInLocation(FullDateFormat, "2026-11-01 01:30:00", loc)
		if name, off := first.Zone(); name != "EDT" || off != -4*3600 {
			t.Errorf("expected EDT/-14400 for first pass of fold, got %s/%d", name, off)
		}
		second := first.Add(time.Hour)
		if name, off := second.Zone(); name != "EST" || off != -5*3600 {
			t.Errorf("expected EST/-18000 for second pass of fold, got %s/%d", name, off)
		}
		if got := second.In(loc).Format(FullDateFormat); got != "2026-11-01 01:30:00" {
			t.Errorf("fold should repeat wall clock 2026-11-01 01:30:00, got %s", got)
		}
	})

	t.Run("deterministic data", func(t *testing.T) {
		a, b := conformanceTZData(), conformanceTZData()
		if len(a) != len(b) {
			t.Fatalf("tz data length unstable: %d vs %d", len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("tz data differs at byte %d", i)
			}
		}
	})
}
