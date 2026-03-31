package backend

import "sync"

// CaptureScreens runs CaptureScreen concurrently for all containers.
// The sessions map is containerID -> tmux session name.
func CaptureScreens(b Backend, sessions map[string]string) map[string]ScreenCapture {
	result := make(map[string]ScreenCapture, len(sessions))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for id, sess := range sessions {
		wg.Add(1)
		go func(cid, session string) {
			defer wg.Done()
			sc := b.CaptureScreen(cid, session)
			mu.Lock()
			result[cid] = sc
			mu.Unlock()
		}(id, sess)
	}

	wg.Wait()
	return result
}

// AgentToolProbes runs AgentToolProbe concurrently for all containers.
// Containers whose probe succeeded are in the result (even if no agent
// was found — stored as empty string). Containers whose probe failed
// are omitted so the caller can preserve their previous state.
func AgentToolProbes(b Backend, containerIDs []string) map[string]string {
	result := make(map[string]string, len(containerIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range containerIDs {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			tool, ok := b.AgentToolProbe(cid)
			mu.Lock()
			if ok {
				result[cid] = tool
			}
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return result
}
