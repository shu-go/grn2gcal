package main

import (
	"time"
)

func FirstDayOfMonth(dt time.Time) time.Time {
	y, m, _ := dt.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	return first
}

func LastDayOfMonth(dt time.Time) time.Time {
	y, m, _ := dt.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	return last
}
