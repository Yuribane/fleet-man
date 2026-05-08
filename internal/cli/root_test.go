package cli

import (
	"strings"
	"testing"
)

func TestPasteClipboardCommandWSL(t *testing.T) {
	got := pasteClipboardCommandFor(true)
	if !strings.Contains(got, "powershell.exe") || !strings.Contains(got, "Get-Clipboard") {
		t.Fatalf("WSL paste command = %q, want PowerShell Get-Clipboard", got)
	}
}

func TestPasteClipboardCommandLinux(t *testing.T) {
	got := pasteClipboardCommandFor(false)
	for _, want := range []string{"wl-paste", "xsel", "xclip", "pbpaste"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Linux paste command = %q, want %q fallback", got, want)
		}
	}
}
