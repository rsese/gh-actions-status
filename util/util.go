package util

import (
	"fmt"
	"time"
)

func Pluralize(num int, thing string) string {
	if num == 1 {
		return fmt.Sprintf("%d %s", num, thing)
	}
	return fmt.Sprintf("%d %ss", num, thing)
}

func FuzzyAgo(ago time.Duration) string {
	if ago < 24*time.Hour {
		return Pluralize(int(ago.Hours()), "hour")
	}
	if ago < 30*24*time.Hour {
		return Pluralize(int(ago.Hours())/24, "day")
	}
	if ago < 365*24*time.Hour {
		return Pluralize(int(ago.Hours())/24/30, "month")
	}

	return Pluralize(int(ago.Hours()/24/365), "year")
}
