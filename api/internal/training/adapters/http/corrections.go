package http

import (
	nethttp "net/http"

	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	"github.com/hectorruiz23/temper/api/internal/training/app"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (h *Handler) CorrectSet(w nethttp.ResponseWriter, request *nethttp.Request) {
	body, err := readBody(request)
	if err != nil {
		writeBodyError(w, err)
		return
	}

	var decoded correctSetRequest
	if err := decodeJSON(body, &decoded); err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}

	if decoded.ExpectedRevision == nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "expected_revision is required")
		return
	}
	if decoded.Changes == nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "changes is required")
		return
	}

	result, err := h.service.CorrectSet(request.Context(), app.CorrectSetCommand{
		IdempotencyKey:   request.Header.Get("Idempotency-Key"),
		SetID:            request.PathValue("id"),
		ExpectedRevision: *decoded.ExpectedRevision,
		Reason:           decoded.Reason,
		Changes: domain.SetChanges{
			ExerciseID: decoded.Changes.ExerciseID,
			SetIndex:   decoded.Changes.SetIndex,
			KG:         decoded.Changes.KG,
			Reps:       decoded.Changes.Reps,
			Note:       decoded.Changes.Note,
		},
	})
	if err != nil {
		writeServiceError(w, err, nethttp.StatusBadRequest)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, correctionResponse{
		Set:          setResponseFrom(result.Set),
		CorrectionID: result.CorrectionID,
		Echo: correctionEchoResponse{
			ExpectedRevision: result.Echo.ExpectedRevision,
			Reason:           result.Echo.Reason,
			Changes: setChangesResponse{
				ExerciseID: result.Echo.Changes.ExerciseID,
				SetIndex:   result.Echo.Changes.SetIndex,
				KG:         result.Echo.Changes.KG,
				Reps:       result.Echo.Changes.Reps,
				Note:       result.Echo.Changes.Note,
			},
		},
	})
}

func (h *Handler) Progression(w nethttp.ResponseWriter, request *nethttp.Request) {
	result, err := h.service.Progression(request.Context(), request.PathValue("exercise_id"))
	if err != nil {
		writeServiceError(w, err, nethttp.StatusBadRequest)
		return
	}

	history := make([]performedSetResponse, len(result.PerformedHistory))
	for index, set := range result.PerformedHistory {
		history[index] = performedSetResponse{
			SetID:          set.SetID,
			SessionID:      set.SessionID,
			Date:           set.Date,
			SetIndex:       set.SetIndex,
			KG:             set.KG,
			Reps:           set.Reps,
			Note:           set.Note,
			EvidenceSource: set.EvidenceSource,
			Revision:       set.Revision,
		}
	}

	targets := make([]exerciseTargetResponse, len(result.CurrentTargets))
	for index, target := range result.CurrentTargets {
		targets[index] = exerciseTargetResponse{
			TemplateID: target.TemplateID,
			TargetSets: target.TargetSets,
			RepsMin:    target.RepsMin,
			RepsMax:    target.RepsMax,
			TargetKG:   target.TargetKG,
			Tempo:      target.Tempo,
		}
	}

	dimensions := make([]progressionDimensionResponse, len(result.ProgressionDimensions))
	for index, dimension := range result.ProgressionDimensions {
		dimensions[index] = progressionDimensionResponse{
			Name:      dimension.Name,
			Dimension: dimension.Dimension,
			Basis:     dimension.Basis,
		}
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, progressionResponse{
		PerformedHistory:      history,
		CurrentTargets:        targets,
		ProgressionDimensions: dimensions,
	})
}

type correctSetRequest struct {
	ExpectedRevision *int64             `json:"expected_revision"`
	Reason           string             `json:"reason"`
	Changes          *setChangesRequest `json:"changes"`
}

type setChangesRequest struct {
	ExerciseID *string  `json:"exercise_id"`
	SetIndex   *int     `json:"set_index"`
	KG         *float64 `json:"kg"`
	Reps       *int     `json:"reps"`
	Note       *string  `json:"note"`
}

type correctionResponse struct {
	Set          setResponse            `json:"set"`
	CorrectionID string                 `json:"correction_id"`
	Echo         correctionEchoResponse `json:"echo"`
}

type correctionEchoResponse struct {
	ExpectedRevision int64              `json:"expected_revision"`
	Reason           string             `json:"reason"`
	Changes          setChangesResponse `json:"changes"`
}

type setChangesResponse struct {
	ExerciseID *string  `json:"exercise_id,omitempty"`
	SetIndex   *int     `json:"set_index,omitempty"`
	KG         *float64 `json:"kg,omitempty"`
	Reps       *int     `json:"reps,omitempty"`
	Note       *string  `json:"note,omitempty"`
}

type progressionResponse struct {
	PerformedHistory      []performedSetResponse         `json:"performed_history"`
	CurrentTargets        []exerciseTargetResponse       `json:"current_targets"`
	ProgressionDimensions []progressionDimensionResponse `json:"progression_dimensions"`
}

type performedSetResponse struct {
	SetID          string  `json:"set_id"`
	SessionID      string  `json:"session_id"`
	Date           string  `json:"date"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	Revision       int64   `json:"revision"`
}

type exerciseTargetResponse struct {
	TemplateID string   `json:"template_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type progressionDimensionResponse struct {
	Name      string `json:"name"`
	Dimension any    `json:"dimension"`
	Basis     string `json:"basis"`
}
