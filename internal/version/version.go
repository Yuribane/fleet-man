package version

// Version is set at build time via -ldflags:
//
//	go build -ldflags "-X github.com/BenjaminBenetti/fleet-man/internal/version.Version=v1.0.0"
//
// When unset (local dev builds), update checks are skipped.
var Version string
