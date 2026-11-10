package server

import (
	"context"

	"github.com/subgate/subgate/core/internal/panel"
)

type panelVersionDiagnostic struct {
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	State    string `json:"state"`
}

func initialPanelVersionDiagnostic() panelVersionDiagnostic {
	return panelVersionDiagnostic{Expected: panel.SupportedVersion, State: "not_checked"}
}

func (s *Server) checkPanelVersion(ctx context.Context) {
	diagnostic := panelVersionDiagnostic{Expected: panel.SupportedVersion, State: "unavailable"}
	actual, err := s.panel.Version(ctx)
	if err == nil {
		diagnostic.Actual = actual
		diagnostic.State = "compatible"
		if !panel.VersionCompatible(actual) {
			diagnostic.State = "mismatch"
			s.logger.Warn("panel version mismatch", "expected", panel.SupportedVersion, "actual", actual)
		}
	} else {
		s.logger.Warn("panel version check unavailable")
	}
	s.stateMu.Lock()
	s.panelVersion = diagnostic
	s.stateMu.Unlock()
}
