package devcontainer

import (
	"strconv"
	"strings"
)

// toolProbeScript is the shell script run inside each container to
// detect which agent tool is running.
const toolProbeScript = `
for t in claude copilot codex gemini; do
  pids=$(ps aux 2>/dev/null | awk -v t="$t" '($11 ~ "(^|/)"t"$" || $12 ~ "(^|/)"t"$") {print $2}')
  [ -n "$pids" ] && { echo "$t"; exit 0; }
done
echo "-"
`

// parseToolProbeOutput parses the tool probe script output.
func parseToolProbeOutput(output string) (string, bool) {
	tool := strings.TrimSpace(output)
	if tool == "" {
		return "", false
	}
	if tool == "-" {
		return "", true
	}
	return tool, true
}

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
