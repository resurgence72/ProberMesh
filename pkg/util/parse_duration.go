package util

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationRE = regexp.MustCompile("^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$")

func ParseDuration(durationStr string) (time.Duration, error) {
	switch durationStr {
	case "0":
		// Allow 0 without a unit.
		return 0, nil
	case "":
		return 0, fmt.Errorf("empty duration string")
	}
	matches := durationRE.FindStringSubmatch(durationStr)
	if matches == nil {
		return 0, fmt.Errorf("not a valid duration string: %q", durationStr)
	}
	var dur time.Duration

	// Parse the match at pos `pos` in the regex and use `mult` to turn that
	// into ms, then add that value to the total parsed duration.
	m := func(pos int, mult time.Duration) {
		if matches[pos] == "" {
			return
		}
		n, _ := strconv.Atoi(matches[pos])
		d := time.Duration(n) * time.Millisecond
		dur += d * mult
	}

	m(2, 1000*60*60*24*365) // y
	m(4, 1000*60*60*24*7)   // w
	m(6, 1000*60*60*24)     // d
	m(8, 1000*60*60)        // h
	m(10, 1000*60)          // m
	m(12, 1000)             // s
	m(14, 1)                // ms

	return dur, nil
}
