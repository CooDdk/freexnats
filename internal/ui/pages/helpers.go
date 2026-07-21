package pages

import (
	"time"

	"github.com/CooDdk/freexnats/pkg/utils"
)

func formatMax(v int64) string {
	if v < 0 {
		return "unlimited"
	}
	return utils.FormatNumber(uint64(v))
}

func formatMaxBytes(v int64) string {
	if v < 0 {
		return "unlimited"
	}
	return utils.FormatBytes(uint64(v))
}

func formatMaxAge(d time.Duration) string {
	if d == 0 {
		return "unlimited"
	}
	return utils.FormatDuration(d)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
