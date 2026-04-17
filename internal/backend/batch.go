package backend

import "sync"

// CaptureAllSessionsForAll runs CaptureAllSessions concurrently for
// every container. Returns a map keyed by containerID. Entries with
// OK=false indicate the container's exec failed and the previous
// activity state should be preserved by the caller.
func CaptureAllSessionsForAll(b Backend, containerIDs []string) map[string]AllSessions {
	result := make(map[string]AllSessions, len(containerIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range containerIDs {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			all := b.CaptureAllSessions(cid)
			mu.Lock()
			result[cid] = all
			mu.Unlock()
		}(id)
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
