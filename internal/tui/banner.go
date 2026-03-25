package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Character map for simple block letters — each char is 5 lines tall.
var charMap = map[rune][5]string{
	'a': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__,_|"},
	'b': {" _     ", "| |__  ", "| '_ \\ ", "| |_) |", "|_.__/ "},
	'c': {"      ", "  ___ ", " / __|", "| (__ ", " \\___|"},
	'd': {"     _ ", "  __| |", " / _` |", "| (_| |", " \\__,_|"},
	'e': {"      ", "  ___ ", " / _ \\", "|  __/", " \\___|"},
	'f': {" __ ", "/ _|", "| | ", "| | ", "|_| "},
	'g': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__, |"},
	'h': {" _     ", "| |__  ", "| '_ \\ ", "| | | |", "|_| |_|"},
	'i': {" _ ", "(_)", "| |", "| |", "|_|"},
	'j': {"   _ ", "  (_)", "  | |", "  | |", " _/ |"},
	'k': {" _    ", "| | __", "| |/ /", "|   < ", "|_|\\_\\"},
	'l': {" _ ", "| |", "| |", "| |", "|_|"},
	'm': {"            ", " _ __ ___   ", "| '_ ` _ \\  ", "| | | | | | ", "|_| |_| |_| "},
	'n': {"        ", " _ __   ", "| '_ \\  ", "| | | | ", "|_| |_| "},
	'o': {"       ", "  ___  ", " / _ \\ ", "| (_) |", " \\___/ "},
	'p': {"       ", " _ __  ", "| '_ \\ ", "| |_) |", "| .__/ "},
	'q': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__, |"},
	'r': {"      ", " _ __ ", "| '__|", "| |   ", "|_|   "},
	's': {"     ", " ___ ", "/ __|", "\\__ \\", "|___/"},
	't': {" _   ", "| |_ ", "| __|", "| |_ ", " \\__|"},
	'u': {"       ", " _   _ ", "| | | |", "| |_| |", " \\__,_|"},
	'v': {"       ", "__   __", "\\ \\ / /", " \\ V / ", "  \\_/  "},
	'w': {"              ", "__      __    ", "\\ \\ /\\ / /    ", " \\ V  V /     ", "  \\_/\\_/      "},
	'x': {"      ", "__  __", "\\ \\/ /", " >  < ", "/_/\\_\\"},
	'y': {"       ", " _   _ ", "| | | |", "| |_| |", " \\__, |"},
	'z': {"     ", " ____", "|_  /", " / / ", "/___|"},
	'0': {"  ___  ", " / _ \\ ", "| | | |", "| |_| |", " \\___/ "},
	'1': {" _ ", "/ |", "| |", "| |", "|_|"},
	'2': {" ____  ", "|___ \\ ", "  __) |", " / __/ ", "|_____|"},
	'3': {" _____ ", "|___ / ", "  |_ \\ ", " ___) |", "|____/ "},
	'4': {" _  _   ", "| || |  ", "| || |_ ", "|__   _|", "   |_|  "},
	'5': {" ____  ", "| ___| ", "|___ \\ ", " ___) |", "|____/ "},
	'6': {"  __   ", " / /_  ", "| '_ \\ ", "| (_) |", " \\___/ "},
	'7': {" _____ ", "|___  |", "   / / ", "  / /  ", " /_/   "},
	'8': {"  ___  ", " ( _ ) ", " / _ \\ ", "| (_) |", " \\___/ "},
	'9': {"  ___  ", " / _ \\ ", "| (_) |", " \\__, |", "   /_/ "},
	'-': {"     ", "     ", " ___ ", "|___|", "     "},
	'/': {"    __", "   / /", "  / / ", " / /  ", "/_/   "},
	'.': {"   ", "   ", "   ", " _ ", "(_)"},
	'_': {"      ", "      ", "      ", " ____ ", "|____|"},
	' ': {"  ", "  ", "  ", "  ", "  "},
}

// nameToBanner converts a string into 5-line ASCII art.
func nameToBanner(name string) string {
	name = strings.ToLower(name)
	lines := [5]strings.Builder{}

	for _, ch := range name {
		glyph, ok := charMap[ch]
		if !ok {
			glyph = charMap[' ']
		}
		for i := 0; i < 5; i++ {
			lines[i].WriteString(glyph[i])
		}
	}

	var result strings.Builder
	for i := 0; i < 5; i++ {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(lines[i].String())
	}
	return result.String()
}

// execWithBanner returns an *exec.Cmd that clears the screen, prints a banner, then execs the given command.
func execWithBanner(banner string, name string, args ...string) *exec.Cmd {
	// Write the banner to a temp file so the shell can cat it
	f, err := os.CreateTemp("", "fleet-banner-*.txt")
	if err != nil {
		return exec.Command(name, args...)
	}
	fmt.Fprintln(f, banner)
	fmt.Fprintln(f)
	f.Close()

	// Build a shell command: clear, print banner, exec into container
	shellArgs := []string{name}
	shellArgs = append(shellArgs, args...)
	quoted := make([]string, len(shellArgs))
	for i, a := range shellArgs {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}

	script := fmt.Sprintf(
		`clear; cat '%s'; rm -f '%s'; exec %s`,
		f.Name(), f.Name(), strings.Join(quoted, " "),
	)

	return exec.Command("sh", "-c", script)
}
