package server

import (
	"context"
	"net/http"

	"github.com/subgate/subgate/core/internal/notify"
)

func (s *Server) notificationTest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Category string          `json:"category"`
		Severity notify.Severity `json:"severity"`
		Title    string          `json:"title"`
		Message  string          `json:"message"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Category == "" {
		input.Category = "system"
	}
	if input.Title == "" {
		input.Title = "SubGate test notification"
	}
	err := s.notify.Send(context.WithoutCancel(r.Context()), notify.Alert{Category: input.Category, Severity: input.Severity, Title: input.Title, Message: input.Message, Dedupe: "manual-test-" + input.Category})
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
