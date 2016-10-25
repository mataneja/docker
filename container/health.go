package container

import "github.com/docker/docker/api/types"

// Health holds the current container health-check state
type Health types.Health

func (s *Health) copy() *Health {
	var copy Health
	copy = *s
	return &copy
}

// String returns a human-readable description of the health-check state
func (s *Health) String() string {
	switch s.Status {
	case types.Starting:
		return "health: starting"
	default: // Healthy and Unhealthy are clear on their own
		return s.Status
	}
}
