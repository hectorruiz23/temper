package http

import (
	nethttp "net/http"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
)

const exercisePageSize = 50

type CatalogueHandler struct {
	service *app.CatalogueService
}

func NewCatalogueHandler(service *app.CatalogueService) *CatalogueHandler {
	return &CatalogueHandler{service: service}
}

func (h *CatalogueHandler) Inventory(w nethttp.ResponseWriter, request *nethttp.Request) {
	items, err := h.service.GetInventory(request.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	response := inventoryResponse{Items: make([]inventoryItemResponse, len(items))}
	for index, item := range items {
		response.Items[index] = inventoryItemResponse{
			EquipmentID: item.EquipmentID,
			Name:        item.Name,
			Quantity:    item.Quantity,
			MaxKG:       item.MaxKG,
		}
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, response)
}

func (h *CatalogueHandler) List(w nethttp.ResponseWriter, request *nethttp.Request) {
	exercises, err := h.service.ListExercises(request.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	cursor := request.URL.Query().Get("cursor")
	start := 0
	if cursor != "" {
		found := false
		for index, exercise := range exercises {
			if exercise.ID == cursor {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "cursor does not reference a known exercise")
			return
		}
	}

	end := start + exercisePageSize
	if end > len(exercises) {
		end = len(exercises)
	}

	page := exercises[start:end]
	if page == nil {
		page = make([]domain.Exercise, 0)
	}

	response := exerciseListResponse{Exercises: page}
	if end < len(exercises) {
		next := exercises[end-1].ID
		response.NextCursor = &next
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, response)
}

func (h *CatalogueHandler) Create(w nethttp.ResponseWriter, request *nethttp.Request) {
	key := request.Header.Get("Idempotency-Key")
	if len(key) == 0 || len(key) > 128 {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
		return
	}

	var input exerciseRequest
	if !readJSONBody(w, request, &input) {
		return
	}

	exercise, err := h.service.CreateExercise(request.Context(), app.CreateExerciseCommand{
		ID:              slugify(input.Name),
		Name:            input.Name,
		MovementPattern: input.MovementPattern,
		Unilateral:      input.Unilateral,
		Equipment:       input.Equipment,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusCreated, exerciseEnvelope{Exercise: exercise})
}

func slugify(name string) string {
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(strings.TrimSpace(name)) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			separator = false
			continue
		}
		separator = true
	}
	return result.String()
}

type inventoryResponse struct {
	Items []inventoryItemResponse `json:"items"`
}

type inventoryItemResponse struct {
	EquipmentID string   `json:"equipment_id"`
	Name        string   `json:"name"`
	Quantity    int      `json:"quantity"`
	MaxKG       *float64 `json:"max_kg"`
}

type exerciseRequest struct {
	Name            string                     `json:"name"`
	MovementPattern string                     `json:"movement_pattern"`
	Unilateral      bool                       `json:"unilateral"`
	Equipment       []domain.ExerciseEquipment `json:"equipment"`
}

type exerciseEnvelope struct {
	Exercise domain.Exercise `json:"exercise"`
}

type exerciseListResponse struct {
	Exercises  []domain.Exercise `json:"exercises"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}
