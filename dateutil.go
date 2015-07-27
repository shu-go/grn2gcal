package main

import (
	"time"
)

// FirstDayOfMonth ...
// 2015/02/03 -> 2015/02/01
func FirstDayOfMonth(dt time.Time) time.Time {
	y, m, _ := dt.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	return first
}

// LastDayOfMonth ...
// 2015/02/03 -> 2015/02/28
func LastDayOfMonth(dt time.Time) time.Time {
	y, m, _ := dt.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	return last
}
