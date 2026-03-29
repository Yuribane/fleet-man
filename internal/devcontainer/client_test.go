package devcontainer

import "testing"

func TestScreenCaptureZeroValue(t *testing.T) {
	// A zero-value ScreenCapture should indicate failure (OK=false).
	var sc ScreenCapture
	if sc.OK {
		t.Fatal("zero-value ScreenCapture should have OK=false")
	}
	if sc.Content != "" {
		t.Fatal("zero-value ScreenCapture should have empty Content")
	}
}
