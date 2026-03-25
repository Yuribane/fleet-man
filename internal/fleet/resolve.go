package fleet

import (
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"
)

// Target represents a resolved fleet + instance name pair.
type Target struct {
	Fleet    string
	Instance string
}

// Resolve parses a user-provided name into a fleet/instance target.
// It handles three forms:
//   - "instance"           → fleet inferred from cwd git remote
//   - "fleet/instance"     → explicit fleet name
//   - "instance" + repoURL → fleet derived from the repo URL
func Resolve(name string, repoFlag string) (*Target, error) {
	if repoFlag != "" {
		fleetName := FleetNameFromRemote(repoFlag)
		if fleetName == "" {
			return nil, fmt.Errorf("could not derive fleet name from repo URL %q", repoFlag)
		}
		return &Target{Fleet: fleetName, Instance: name}, nil
	}

	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target %q: both fleet and instance names are required", name)
		}
		return &Target{Fleet: parts[0], Instance: parts[1]}, nil
	}

	// Infer fleet from cwd git remote
	fleetName, err := FleetNameFromCwd()
	if err != nil {
		return nil, fmt.Errorf("could not infer fleet from cwd: %w (use fleet/instance or --repo)", err)
	}
	return &Target{Fleet: fleetName, Instance: name}, nil
}

// FleetNameFromCwd reads the git remote origin URL from the current directory
// and derives a fleet name from it.
func FleetNameFromCwd() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repo or no origin remote: %w", err)
	}
	remote := strings.TrimSpace(string(out))
	name := FleetNameFromRemote(remote)
	if name == "" {
		return "", fmt.Errorf("could not parse fleet name from remote %q", remote)
	}
	return name, nil
}

// FleetNameFromRemote extracts a fleet name from a git remote URL.
// Examples:
//
//	git@github.com:org/fleet-man.git → fleet-man
//	https://github.com/org/fleet-man.git → fleet-man
//	https://github.com/org/fleet-man → fleet-man
func FleetNameFromRemote(remote string) string {
	remote = strings.TrimSpace(remote)

	// Handle SSH-style: git@github.com:org/repo.git
	if strings.Contains(remote, ":") && !strings.Contains(remote, "://") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			remote = parts[1]
		}
	} else {
		// Handle HTTPS-style URLs
		parsed, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		remote = parsed.Path
	}

	// Get the last path component and strip .git suffix
	name := path.Base(remote)
	name = strings.TrimSuffix(name, ".git")
	return name
}

// RemoteURLFromCwd reads the git remote origin URL from the current directory.
func RemoteURLFromCwd() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repo or no origin remote: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
