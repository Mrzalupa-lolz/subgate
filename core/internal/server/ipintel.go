package server

import "net/http"

func (s *Server) reloadIntel(w http.ResponseWriter, r *http.Request) {
	if s.config.IPIntel.DataPath == "" {
		fail(w, 409, "IP intelligence data path is not configured")
		return
	}
	if err := s.auditAction(r, "ipintel.reload", "ipintel", s.config.IPIntel.DataPath); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	if err := s.intel.LoadPrefixes(s.config.IPIntel.DataPath); err != nil {
		fail(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "health": s.intel.Health()})
}
