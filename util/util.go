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

func PrettyMS(ms int) string {
	if ms == 60000 {
		return "1m"
	}
	if ms < 60000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fm", float32(ms)/60000)
}
