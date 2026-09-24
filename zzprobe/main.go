package main

import (
	"fmt"
	"time"

	gronx "github.com/adhocore/gronx"
)

func main() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	g := gronx.New()

	ref := time.Date(2024, 11, 3, 0, 30, 0, 0, loc)
	n, err := gronx.NextTickAfter("0 1 * * *", ref, false)
	fmt.Printf("fold next 0 1 from 00:30 -> %v off=%d err=%v\n", n, off(n), err)
	for i := 0; i < 3; i++ {
		n, err = gronx.NextTickAfter("0 1 * * *", n, false)
		fmt.Printf("  then -> %v off=%d err=%v\n", n, off(n), err)
	}
	t1 := time.Date(2024, 11, 3, 1, 0, 0, 0, loc)
	due, derr := g.IsDue("0 1 * * *", t1)
	fmt.Printf("isDue at first 01:00: due=%v err=%v\n", due, derr)

	gref := time.Date(2024, 3, 10, 1, 30, 0, 0, loc)
	gn, gerr := gronx.NextTickAfter("30 2 * * *", gref, false)
	fmt.Printf("gap next 2:30 from 01:30 -> %v off=%d err=%v\n", gn, off(gn), gerr)
	gn2, gerr2 := gronx.NextTickAfter("30 2 * * *", gn, false)
	fmt.Printf("  then -> %v off=%d err=%v\n", gn2, off(gn2), gerr2)

	// minute cron across fold: enumerate instants from first 01:00 to 02:00
	cur := time.Date(2024, 11, 3, 1, 0, 0, 0, loc)
	end := time.Date(2024, 11, 3, 2, 0, 0, 0, loc)
	count := 0
	for cur.Before(end) && count < 200 {
		nn, e := gronx.NextTickAfter("* 1 * * *", cur, false)
		if e != nil {
			fmt.Println("iter err", e)
			break
		}
		if !nn.After(cur) {
			fmt.Println("stall at", cur)
			break
		}
		cur = nn
		count++
	}
	fmt.Printf("minute occurrences in [01:00,02:00): %d (expect 120 for fold double-hour)\n", count)

	// prev from end should reach second 01:59 (EST)
	p, perr := gronx.PrevTickBefore("* 1 * * *", end, false)
	fmt.Printf("prev before 02:00 -> %v off=%d err=%v (expect 01:59 EST off=-18000)\n", p, off(p), perr)
}

func off(t time.Time) int {
	_, o := t.Zone()
	return o
}
