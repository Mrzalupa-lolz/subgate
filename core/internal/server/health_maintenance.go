package server

import "time"

// Maintenance is configured by the owner in YAML and applied after a restart.
// A window is [from,until) in absolute time; overlapping windows are a union.
func (s *Server) inMaintenance(nodeID string, at time.Time) bool {
	for _, window := range s.config.Health.MaintenanceWindows {
		if window.NodeID == nodeID && !at.Before(window.From) && at.Before(window.Until) {
			return true
		}
	}
	return false
}

func (s *Server) overlapsMaintenance(nodeID string, from, until time.Time) bool {
	if until.IsZero() || until.Equal(from) {
		return s.inMaintenance(nodeID, from)
	}
	for _, window := range s.config.Health.MaintenanceWindows {
		if window.NodeID == nodeID && from.Before(window.Until) && until.After(window.From) {
			return true
		}
	}
	return false
}
