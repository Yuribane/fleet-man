package backend

// ContainerStats holds CPU and memory usage for a container.
type ContainerStats struct {
	CPUMillicores float64
	MemoryMB      float64
}
