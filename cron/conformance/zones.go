package conformance

import (
	"encoding/binary"
	"time"
)

// This file constructs the embedded, deterministic timezone test data used by
// the conformance corpus. Nothing here reads /etc/localtime, the TZ
// environment variable or any machine-provided zone database: the zone bytes
// are a minimal TZif v1 stream assembled from fixed constants and loaded
// through the standard library public entry time.LoadLocationFromTZData.

// ZoneGapFold is the name of the embedded conformance zone.
const ZoneGapFold = "Conformance/GapFold"

// tzif abbreviation table indices.
const (
	stdAbbr = "TST\x00"
	dstAbbr = "TSU\x00"
)

// zoneTZif returns a minimal TZif v1 stream describing a zone whose wall clock
// is UTC+2 (TST) in winter and UTC+3 (TSU) in summer. In 2021 the transitions
// are:
//
//	2021-03-28 01:00 UTC  standard(+2) -> summer(+3): wall 02:00 jumps to 03:00
//	                     so the wall interval [02:00,03:00) never exists (gap).
//	2021-10-31 01:00 UTC  summer(+3) -> standard(+2): wall 03:00 repeats as
//	                     02:00, so the wall interval [02:00,03:00) occurs twice
//	                     (fold): first at UTC 00:00-01:00 (+3), then at
//	                     UTC 01:00-02:00 (+2).
//
// Only these two transitions are encoded; wall times outside their window are
// interpreted with the resulting fixed offset, which is exactly what the
// corpus cases need.
func zoneTZif() []byte {
	gap := time.Date(2021, 3, 28, 1, 0, 0, 0, time.UTC).Unix()
	fold := time.Date(2021, 10, 31, 1, 0, 0, 0, time.UTC).Unix()

	abbrs := stdAbbr + dstAbbr

	buf := make([]byte, 0, 4+15+24+8+2+12+len(abbrs))
	// Magic "TZif" + version NUL (v1) + 15 reserved bytes.
	buf = append(buf, 'T', 'Z', 'i', 'f', 0)
	buf = append(buf, make([]byte, 15)...)

	header := make([]byte, 24)
	// tzh_ttisgmtcnt=0, tzh_ttisstdcnt=0, tzh_leapcnt=0
	binary.BigEndian.PutUint32(header[12:], 2) // tzh_timecnt
	binary.BigEndian.PutUint32(header[16:], 2) // tzh_typecnt
	binary.BigEndian.PutUint32(header[20:], uint32(len(abbrs)))
	buf = append(buf, header...)

	for _, transition := range []int64{gap, fold} {
		octets := make([]byte, 4)
		binary.BigEndian.PutUint32(octets, uint32(transition))
		buf = append(buf, octets...)
	}
	buf = append(buf, 1, 0) // first transition -> type 1 (dst), second -> type 0 (std)

	ttinfo := func(offset int32, isDST byte, abbrIndex byte) []byte {
		record := make([]byte, 6)
		binary.BigEndian.PutUint32(record[0:], uint32(offset))
		record[4] = isDST
		record[5] = abbrIndex
		return record
	}
	buf = append(buf, ttinfo(2*3600, 0, 0)...) // type 0: TST +2
	buf = append(buf, ttinfo(3*3600, 1, 4)...) // type 1: TSU +3

	buf = append(buf, abbrs...)

	// POSIX trailing string (newline delimited); it documents the same rules
	// and keeps the stream valid for readers that parse the footer.
	buf = append(buf, '\n')
	buf = append(buf, "TST-2TSU-3,M3.5.0/1,M10.5.0/1"...)
	buf = append(buf, '\n')
	return buf
}

var gapFoldZone = mustLoadZone()

func mustLoadZone() *time.Location {
	loc, err := time.LoadLocationFromTZData(ZoneGapFold, zoneTZif())
	if err != nil {
		panic("gronx conformance: invalid embedded tzif data: " + err.Error())
	}
	return loc
}

// utcZone is the embedded fixed-offset zone used for standard-clear cases.
func utcZone() *time.Location { return time.UTC }
