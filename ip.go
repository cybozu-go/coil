package coil

import "time"

// IPAssignment holds IP address assignment information for a pod/container
type IPAssignment struct {
	ContainerID string    `json:"container_id"`
	Namespace   string    `json:"namespace"`
	Pod         string    `json:"pod"`
	CreatedAt   time.Time `json:"created_at"`
}
