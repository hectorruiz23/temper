package http

import (
	"errors"
	nethttp "net/http"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
)

func writeServiceError(w nethttp.ResponseWriter, err error) {
	var rule kernel.RuleError
	if !errors.As(err, &rule) {
		httpserver.WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	status := statusForRule(rule.Code)
	if status == nethttp.StatusInternalServerError {
		httpserver.WriteError(w, status, "internal_error", "internal error")
		return
	}
	httpserver.WriteError(w, status, rule.Code, rule.Message)
}

func statusForRule(code string) int {
	switch code {
	case "invalid_input", "invalid_date", "idempotency_key_required":
		return nethttp.StatusBadRequest
	case "not_found":
		return nethttp.StatusNotFound
	case "idempotency_conflict", "exercise_exists", "session_id_taken", "session_position_taken":
		return nethttp.StatusConflict
	case "unknown_exercise", "unavailable_equipment", "equipment_limit", "invalid_schedule", "invalid_target", "invalid_interval_steps", "unknown_equipment":
		return nethttp.StatusUnprocessableEntity
	default:
		return nethttp.StatusInternalServerError
	}
}
