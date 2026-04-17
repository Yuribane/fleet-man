package devcontainer

import (
	"strconv"
	"strings"
)

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func parseMemToMB(s string) float64 {
	s = strings.TrimSpace(s)
	multiplier := 1.0

	if strings.HasSuffix(s, "GiB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "GiB")
	} else if strings.HasSuffix(s, "MiB") {
		multiplier = 1
		s = strings.TrimSuffix(s, "MiB")
	} else if strings.HasSuffix(s, "KiB") {
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "KiB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1.0 / (1024 * 1024)
		s = strings.TrimSuffix(s, "B")
	}

	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return val * multiplier
}
