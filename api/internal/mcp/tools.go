package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	healthapp "github.com/hectorruiz23/temper/api/internal/health/app"
	healthdomain "github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	plansdomain "github.com/hectorruiz23/temper/api/internal/plans/domain"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
	trainingdomain "github.com/hectorruiz23/temper/api/internal/training/domain"
)

const exercisePageSize = 50

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError"`
}

var toolCatalog = []toolDefinition{
	{
		Name:        "get_inventory",
		Description: "Read the equipment stock.",
		InputSchema: objectSchema(map[string]any{}),
	},
	{
		Name:        "list_exercises",
		Description: "List exercises in the catalogue, optionally after a cursor.",
		InputSchema: objectSchema(map[string]any{
			"cursor": stringSchema("Exercise id to continue after."),
		}),
	},
	{
		Name:        "create_exercise",
		Description: "Add an exercise to the catalogue.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key":  stringSchema("Client key for this write."),
			"name":             stringSchema("Exercise name."),
			"movement_pattern": stringSchema("Movement pattern, for example squat."),
			"unilateral":       booleanSchema("Whether the exercise is performed one side at a time."),
			"equipment": arraySchema("Equipment used by the exercise.", objectSchema(map[string]any{
				"equipment_id": stringSchema("Equipment id."),
				"quantity":     integerSchema("Equipment quantity."),
			}, "equipment_id", "quantity")),
		}, "idempotency_key", "name", "movement_pattern"),
	},
	{
		Name:        "create_plan",
		Description: "Create an immutable plan version.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key":  stringSchema("Client key for this write."),
			"reason":           stringSchema("Why this version exists."),
			"effective_from":   stringSchema("First date the version applies."),
			"timezone":         stringSchema("IANA timezone."),
			"weekly_frequency": integerSchema("Planned sessions per week."),
			"goal_id":          stringSchema("Optional goal id."),
			"sessions":         arraySchema("Sessions in the plan.", planSessionSchema()),
		}, "idempotency_key", "reason", "effective_from", "timezone", "weekly_frequency"),
	},
	{
		Name:        "get_current_plan",
		Description: "Read the plan version in force.",
		InputSchema: objectSchema(map[string]any{}),
	},
	{
		Name:        "get_plan",
		Description: "Read one plan version.",
		InputSchema: objectSchema(map[string]any{
			"id": integerSchema("Plan version id."),
		}, "id"),
	},
	{
		Name:        "get_week",
		Description: "Expand the Monday to Sunday week containing a date.",
		InputSchema: objectSchema(map[string]any{
			"date": stringSchema("ISO date inside the week."),
		}, "date"),
	},
	{
		Name:        "get_day",
		Description: "Read one daily health record.",
		InputSchema: objectSchema(map[string]any{
			"date": stringSchema("ISO date."),
		}, "date"),
	},
	{
		Name:        "upsert_day",
		Description: "Create or update a daily health record.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key":   stringSchema("Client key for this write."),
			"date":              stringSchema("ISO date."),
			"timezone":          stringSchema("IANA timezone."),
			"expected_revision": integerSchema("Revision the caller last saw."),
			"source":            stringSchema("manual or health_connect."),
			"steps_source":      stringSchema("Device origin of the step count: watch, fitness_band, ring, chest_strap, phone, smart_display, head_mounted, scale or other."),
			"metrics": objectSchema(map[string]any{
				"sleep_ms":       integerSchema("Sleep duration in milliseconds."),
				"steps":          integerSchema("Step count."),
				"resting_hr_bpm": integerSchema("Resting heart rate."),
				"hrv_rmssd_ms":   numberSchema("HRV RMSSD in milliseconds."),
			}),
			"sleep_stages": arraySchema("Sleep stages for the day.", objectSchema(map[string]any{
				"stage":       stringSchema("Stage name."),
				"duration_ms": integerSchema("Stage duration in milliseconds."),
			}, "stage", "duration_ms")),
			"workouts": arraySchema("Workouts for the day.", workoutSchema()),
		}, "idempotency_key", "date", "timezone", "expected_revision", "source"),
	},
	{
		Name:        "create_session",
		Description: "Create a performed session with no completion evidence.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key": stringSchema("Client key for this write."),
			"id":              stringSchema("Session id."),
			"template_id":     stringSchema("Optional session template id."),
			"date":            stringSchema("ISO date."),
			"discipline":      stringSchema("strength, run or intervals."),
			"started_at":      stringSchema("Optional RFC 3339 start time."),
		}, "idempotency_key", "id", "date", "discipline"),
	},
	{
		Name:        "check_in_session",
		Description: "Attach exactly one completion evidence source to a session.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key":       stringSchema("Client key for this write."),
			"session_id":            stringSchema("Session id."),
			"expected_revision":     integerSchema("Revision the caller last saw."),
			"reported_completed_at": stringSchema("Optional RFC 3339 reported completion time."),
			"watch_workout_id":      stringSchema("Optional watch workout id."),
		}, "idempotency_key", "session_id", "expected_revision"),
	},
	{
		Name:        "log_sets",
		Description: "Log a batch of sets atomically.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key": stringSchema("Client key for this write."),
			"session_id":      stringSchema("Session id."),
			"sets": arraySchema("Sets to log.", objectSchema(map[string]any{
				"exercise_id": stringSchema("Exercise id."),
				"set_index":   integerSchema("One-based set position."),
				"kg":          numberSchema("Weight in kilograms."),
				"reps":        integerSchema("Repetition count."),
				"note":        stringSchema("Optional note."),
			}, "exercise_id", "set_index", "kg", "reps")),
		}, "idempotency_key", "session_id", "sets"),
	},
	{
		Name:        "correct_set",
		Description: "Correct one set, keeping the before and after.",
		InputSchema: objectSchema(map[string]any{
			"idempotency_key":   stringSchema("Client key for this write."),
			"set_id":            stringSchema("Set id."),
			"expected_revision": integerSchema("Revision the caller last saw."),
			"reason":            stringSchema("Why the set is corrected."),
			"changes": objectSchema(map[string]any{
				"exercise_id": stringSchema("New exercise id."),
				"set_index":   integerSchema("New one-based set position."),
				"kg":          numberSchema("New weight in kilograms."),
				"reps":        integerSchema("New repetition count."),
				"note":        stringSchema("New note."),
			}),
		}, "idempotency_key", "set_id", "expected_revision", "reason", "changes"),
	},
	{
		Name:        "get_progression",
		Description: "Read performed history, current targets and progression dimensions.",
		InputSchema: objectSchema(map[string]any{
			"exercise_id": stringSchema("Exercise id."),
		}, "exercise_id"),
	},
}

func (h *Handler) definitions() []toolDefinition {
	return toolCatalog
}

func (h *Handler) callTool(ctx context.Context, name string, raw json.RawMessage) (toolResult, error) {
	definition, ok := findTool(name)
	if !ok {
		return toolResult{}, paramsError{message: "unknown tool: " + name}
	}
	if err := validateArguments(definition.InputSchema, raw); err != nil {
		return toolResult{}, err
	}

	switch name {
	case "get_inventory":
		return h.getInventory(ctx, raw)
	case "list_exercises":
		return h.listExercises(ctx, raw)
	case "create_exercise":
		return h.createExercise(ctx, raw)
	case "create_plan":
		return h.createPlan(ctx, raw)
	case "get_current_plan":
		return h.getCurrentPlan(ctx, raw)
	case "get_plan":
		return h.getPlan(ctx, raw)
	case "get_week":
		return h.getWeek(ctx, raw)
	case "get_day":
		return h.getDay(ctx, raw)
	case "upsert_day":
		return h.upsertDay(ctx, raw)
	case "create_session":
		return h.createSession(ctx, raw)
	case "check_in_session":
		return h.checkInSession(ctx, raw)
	case "log_sets":
		return h.logSets(ctx, raw)
	case "correct_set":
		return h.correctSet(ctx, raw)
	case "get_progression":
		return h.getProgression(ctx, raw)
	default:
		return toolResult{}, paramsError{message: "unknown tool: " + name}
	}
}

func findTool(name string) (toolDefinition, bool) {
	for _, definition := range toolCatalog {
		if definition.Name == name {
			return definition, true
		}
	}
	return toolDefinition{}, false
}

func (h *Handler) getInventory(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct{}
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	items, err := h.catalogue.GetInventory(ctx)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(inventoryResponseFrom(items), fmt.Sprintf("inventory: %d items", len(items))), nil
}

func (h *Handler) listExercises(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args listExercisesArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	exercises, err := h.catalogue.ListExercises(ctx)
	if err != nil {
		return toolFailure(err)
	}

	start := 0
	if args.Cursor != "" {
		found := false
		for index, exercise := range exercises {
			if exercise.ID == args.Cursor {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return toolFailure(kernel.NewRuleError("invalid_input", "cursor does not reference a known exercise"))
		}
	}

	end := start + exercisePageSize
	if end > len(exercises) {
		end = len(exercises)
	}

	page := exercises[start:end]
	if page == nil {
		page = make([]plansdomain.Exercise, 0)
	}

	response := exerciseListResponse{Exercises: page}
	if end < len(exercises) {
		next := exercises[end-1].ID
		response.NextCursor = &next
	}

	return successResult(response, fmt.Sprintf("exercises: %d", len(page))), nil
}

func (h *Handler) createExercise(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args createExerciseArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	equipment := make([]plansdomain.ExerciseEquipment, len(args.Equipment))
	for index, item := range args.Equipment {
		equipment[index] = plansdomain.ExerciseEquipment{EquipmentID: item.EquipmentID, Quantity: item.Quantity}
	}

	exercise, err := h.catalogue.CreateExercise(ctx, plansapp.CreateExerciseCommand{
		ID:              slugify(args.Name),
		Name:            args.Name,
		MovementPattern: args.MovementPattern,
		Unilateral:      args.Unilateral,
		Equipment:       equipment,
	})
	if err != nil {
		return toolFailure(err)
	}

	return successResult(exerciseEnvelope{Exercise: exercise}, fmt.Sprintf("created exercise %s", exercise.ID)), nil
}

func (h *Handler) createPlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args createPlanArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	plan, err := h.plans.CreatePlan(ctx, plansapp.CreatePlanCommand{
		IdempotencyKey: args.IdempotencyKey,
		Plan:           args.domainPlan(),
	})
	if err != nil {
		return toolFailure(err)
	}

	return successResult(planEnvelope{Plan: planResponseFrom(plan)}, fmt.Sprintf("created plan %d effective %s", plan.ID, plan.EffectiveFrom)), nil
}

func (h *Handler) getCurrentPlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct{}
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	plan, err := h.plans.GetCurrentPlan(ctx)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(planEnvelope{Plan: planResponseFrom(plan)}, fmt.Sprintf("current plan %d", plan.ID)), nil
}

func (h *Handler) getPlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args getPlanArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	plan, err := h.plans.GetPlan(ctx, args.ID)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(planEnvelope{Plan: planResponseFrom(plan)}, fmt.Sprintf("plan %d", plan.ID)), nil
}

func (h *Handler) getWeek(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args weekArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	week, err := h.week(ctx, args.Date)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(week, fmt.Sprintf("week %s to %s", week.WeekStart, week.WeekEnd)), nil
}

func (h *Handler) getDay(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args getDayArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	day, err := h.health.GetDay(ctx, args.Date)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(dayEnvelope{Day: dayResponseFrom(day)}, fmt.Sprintf("day %s at revision %d", day.Date, day.Revision)), nil
}

func (h *Handler) upsertDay(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args upsertDayArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	command := healthapp.UpsertDayCommand{
		Date:             args.Date,
		Timezone:         args.Timezone,
		ExpectedRevision: args.ExpectedRevision,
		Source:           args.Source,
		StepsSource:      args.StepsSource,
		IdempotencyKey:   args.IdempotencyKey,
	}
	if args.Metrics != nil {
		command.Metrics = healthdomain.MetricsPatch{
			SleepMS:      healthdomain.OptionalInt64{Set: args.Metrics.SleepMS.Set, Value: args.Metrics.SleepMS.Value},
			Steps:        healthdomain.OptionalInt64{Set: args.Metrics.Steps.Set, Value: args.Metrics.Steps.Value},
			RestingHRBPM: healthdomain.OptionalInt64{Set: args.Metrics.RestingHRBPM.Set, Value: args.Metrics.RestingHRBPM.Value},
			HRVRMSSDMS:   healthdomain.OptionalFloat64{Set: args.Metrics.HRVRMSSDMS.Set, Value: args.Metrics.HRVRMSSDMS.Value},
		}
	}
	if args.SleepStages != nil {
		stages := make([]healthdomain.SleepStage, len(*args.SleepStages))
		for index, stage := range *args.SleepStages {
			stages[index] = healthdomain.SleepStage{Stage: stage.Stage, DurationMS: stage.DurationMS}
		}
		command.SleepStages = &stages
	}
	if args.Workouts != nil {
		workouts := make([]healthdomain.Workout, len(*args.Workouts))
		for index, workout := range *args.Workouts {
			splits := make([]healthdomain.WorkoutSplit, len(workout.Splits))
			for splitIndex, split := range workout.Splits {
				splits[splitIndex] = healthdomain.WorkoutSplit{
					Position:     split.Position,
					Kind:         split.Kind,
					DistanceM:    split.DistanceM,
					DurationMS:   split.DurationMS,
					AvgHRBPM:     split.AvgHRBPM,
					PaceSecPerKM: split.PaceSecPerKM,
				}
			}
			workouts[index] = healthdomain.Workout{
				Origin:         workout.Origin,
				ExternalID:     workout.ExternalID,
				DeviceKind:     workout.DeviceKind,
				StartedAt:      workout.StartedAt,
				EndedAt:        workout.EndedAt,
				Discipline:     workout.Discipline,
				DurationMS:     workout.DurationMS,
				DistanceM:      workout.DistanceM,
				AvgHRBPM:       workout.AvgHRBPM,
				MaxHRBPM:       workout.MaxHRBPM,
				CaloriesKcal:   workout.CaloriesKcal,
				CadenceSPM:     workout.CadenceSPM,
				ElevationGainM: workout.ElevationGainM,
				PaceSecPerKM:   workout.PaceSecPerKM,
				WatchVerified:  workout.WatchVerified,
				Present:        true,
				Splits:         splits,
			}
		}
		command.Workouts = &workouts
	}

	day, err := h.health.UpsertDay(ctx, command)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(dayEnvelope{Day: dayResponseFrom(day)}, fmt.Sprintf("day %s updated at revision %d", day.Date, day.Revision)), nil
}

func (h *Handler) createSession(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args createSessionArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	startedAt, err := parseTimestamp(args.StartedAt, "started_at")
	if err != nil {
		return toolFailure(err)
	}

	session, err := h.training.CreateSession(ctx, trainingapp.CreateSessionCommand{
		IdempotencyKey: args.IdempotencyKey,
		ID:             args.ID,
		TemplateID:     args.TemplateID,
		Date:           args.Date,
		Discipline:     args.Discipline,
		StartedAt:      startedAt,
	})
	if err != nil {
		return toolFailure(err)
	}

	return successResult(sessionEnvelope{Session: sessionResponseFrom(session)}, fmt.Sprintf("created session %s at revision %d", session.ID, session.Revision)), nil
}

func (h *Handler) checkInSession(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args checkInSessionArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	reportedAt, err := parseTimestamp(args.ReportedCompletedAt, "reported_completed_at")
	if err != nil {
		return toolFailure(err)
	}

	result, err := h.training.CheckIn(ctx, trainingapp.CheckInCommand{
		SessionID:           args.SessionID,
		ExpectedRevision:    args.ExpectedRevision,
		ReportedCompletedAt: reportedAt,
		WatchWorkoutID:      args.WatchWorkoutID,
	})
	if err != nil {
		return toolFailure(err)
	}

	return successResult(checkInEnvelope{
		Session:           sessionResponseFrom(result.Session),
		EffectiveEvidence: evidenceResponseFrom(result.Evidence),
	}, fmt.Sprintf("checked in session %s at revision %d", result.Session.ID, result.Session.Revision)), nil
}

func (h *Handler) logSets(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args logSetsArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	sets := make([]trainingdomain.NewSet, len(args.Sets))
	for index, set := range args.Sets {
		if set.KG == nil {
			return toolFailure(kernel.NewRuleError("invalid_input", "kg is required"))
		}
		sets[index] = trainingdomain.NewSet{
			ExerciseID: set.ExerciseID,
			SetIndex:   set.SetIndex,
			KG:         *set.KG,
			Reps:       set.Reps,
			Note:       set.Note,
		}
	}

	result, err := h.training.LogSets(ctx, trainingapp.LogSetsCommand{
		IdempotencyKey: args.IdempotencyKey,
		SessionID:      args.SessionID,
		Sets:           sets,
	})
	if err != nil {
		return toolFailure(err)
	}

	echoSets := make([]echoSetResponse, len(result.Echo.Sets))
	for index, set := range result.Echo.Sets {
		echoSets[index] = echoSetResponse{
			ExerciseID: set.ExerciseID,
			SetIndex:   set.SetIndex,
			KG:         set.KG,
			Reps:       set.Reps,
			Note:       set.Note,
		}
	}
	responseSets := make([]setResponse, len(result.Sets))
	for index, set := range result.Sets {
		responseSets[index] = setResponseFrom(set)
	}

	return successResult(batchResponse{
		Sets: responseSets,
		Echo: batchEchoResponse{SessionID: result.Echo.SessionID, Sets: echoSets},
	}, fmt.Sprintf("logged %d sets for session %s", len(result.Sets), result.Echo.SessionID)), nil
}

func (h *Handler) correctSet(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args correctSetArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}
	if err := requireIdempotencyKey(args.IdempotencyKey); err != nil {
		return toolFailure(err)
	}

	result, err := h.training.CorrectSet(ctx, trainingapp.CorrectSetCommand{
		IdempotencyKey:   args.IdempotencyKey,
		SetID:            args.SetID,
		ExpectedRevision: args.ExpectedRevision,
		Reason:           args.Reason,
		Changes: trainingdomain.SetChanges{
			ExerciseID: args.Changes.ExerciseID,
			SetIndex:   args.Changes.SetIndex,
			KG:         args.Changes.KG,
			Reps:       args.Changes.Reps,
			Note:       args.Changes.Note,
		},
	})
	if err != nil {
		return toolFailure(err)
	}

	return successResult(correctionResponseFrom(result), fmt.Sprintf("corrected set %s at revision %d", result.Set.ID, result.Set.Revision)), nil
}

func (h *Handler) getProgression(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args progressionArgs
	if err := decodeStrict(raw, &args); err != nil {
		return toolResult{}, paramsError{message: "invalid arguments: " + err.Error()}
	}

	result, err := h.training.Progression(ctx, args.ExerciseID)
	if err != nil {
		return toolFailure(err)
	}

	return successResult(result, fmt.Sprintf("progression for %s: %d performed sets", args.ExerciseID, len(result.PerformedHistory))), nil
}

func successResult(value any, text string) toolResult {
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: text}},
		StructuredContent: value,
		IsError:           false,
	}
}

func toolFailure(err error) (toolResult, error) {
	var rule kernel.RuleError
	if errors.Is(err, healthdomain.ErrDayNotFound) {
		rule = kernel.NewRuleError("not_found", "day not found")
	} else if !errors.As(err, &rule) {
		return toolResult{}, err
	}

	return toolResult{
		Content: []toolContent{{Type: "text", Text: rule.Code + ": " + rule.Message}},
		IsError: true,
	}, nil
}

func requireIdempotencyKey(key string) error {
	if len(key) == 0 || len(key) > 128 {
		return kernel.NewRuleError("idempotency_key_required", "idempotency_key is required and must be at most 128 characters")
	}
	return nil
}

func parseTimestamp(value *string, field string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil, kernel.NewRuleError("invalid_input", field+" must be an RFC 3339 date-time")
	}
	millis := parsed.UnixMilli()
	if millis <= 0 {
		return nil, kernel.NewRuleError("invalid_input", field+" must be a positive timestamp")
	}
	return &millis, nil
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

func (args createPlanArgs) domainPlan() plansdomain.Plan {
	sessions := make([]plansdomain.Session, len(args.Sessions))
	for index, session := range args.Sessions {
		exercises := make([]plansdomain.SessionExercise, len(session.Exercises))
		for exerciseIndex, exercise := range session.Exercises {
			exercises[exerciseIndex] = plansdomain.SessionExercise{
				Position:   exercise.Position,
				ExerciseID: exercise.ExerciseID,
				TargetSets: exercise.TargetSets,
				RepsMin:    exercise.RepsMin,
				RepsMax:    exercise.RepsMax,
				TargetKG:   exercise.TargetKG,
				Tempo:      exercise.Tempo,
			}
		}

		steps := make([]plansdomain.IntervalStep, len(session.IntervalSteps))
		for stepIndex, step := range session.IntervalSteps {
			steps[stepIndex] = plansdomain.IntervalStep{
				Position:   step.Position,
				Label:      step.Label,
				DurationMS: step.DurationMS,
				DistanceM:  step.DistanceM,
			}
		}

		sessions[index] = plansdomain.Session{
			ID:            session.ID,
			Position:      session.Position,
			ScheduledDate: session.ScheduledDate,
			Weekday:       session.Weekday,
			Discipline:    session.Discipline,
			Target:        session.Target,
			Exercises:     exercises,
			Steps:         steps,
		}
	}

	return plansdomain.Plan{
		Reason:          args.Reason,
		EffectiveFrom:   args.EffectiveFrom,
		Timezone:        args.Timezone,
		WeeklyFrequency: args.WeeklyFrequency,
		GoalID:          args.GoalID,
		Sessions:        sessions,
	}
}

func planSessionSchema() map[string]any {
	return objectSchema(map[string]any{
		"id":             stringSchema("Session template id."),
		"position":       integerSchema("Order inside the plan."),
		"scheduled_date": stringSchema("Optional exact ISO date."),
		"weekday":        integerSchema("Optional weekday, 1 is Monday."),
		"discipline":     stringSchema("strength, run or intervals."),
		"target":         stringSchema("Session target."),
		"exercises": arraySchema("Strength exercises.", objectSchema(map[string]any{
			"position":    integerSchema("Order inside the session."),
			"exercise_id": stringSchema("Exercise id."),
			"target_sets": integerSchema("Target set count."),
			"reps_min":    integerSchema("Minimum repetitions."),
			"reps_max":    integerSchema("Maximum repetitions."),
			"target_kg":   numberSchema("Target weight in kilograms."),
			"tempo":       stringSchema("Tempo notation."),
		}, "exercise_id")),
		"interval_steps": arraySchema("Interval steps.", objectSchema(map[string]any{
			"position":    integerSchema("Order inside the session."),
			"label":       stringSchema("Step label."),
			"duration_ms": integerSchema("Duration in milliseconds."),
			"distance_m":  numberSchema("Distance in metres."),
		}, "label")),
	}, "id", "discipline")
}

func workoutSchema() map[string]any {
	return objectSchema(map[string]any{
		"origin":           stringSchema("Workout origin."),
		"external_id":      stringSchema("External workout id."),
		"device_kind":      stringSchema("Optional device kind."),
		"started_at":       integerSchema("Start time in milliseconds since epoch."),
		"ended_at":         integerSchema("End time in milliseconds since epoch."),
		"discipline":       stringSchema("Workout discipline."),
		"duration_ms":      integerSchema("Duration in milliseconds."),
		"distance_m":       numberSchema("Optional distance in metres."),
		"avg_hr_bpm":       integerSchema("Optional average heart rate."),
		"max_hr_bpm":       integerSchema("Optional maximum heart rate."),
		"calories_kcal":    numberSchema("Optional energy expenditure in kilocalories."),
		"cadence_spm":      numberSchema("Optional cadence in steps or revolutions per minute."),
		"elevation_gain_m": numberSchema("Optional elevation gain in metres."),
		"pace_sec_per_km":  numberSchema("Optional pace. The server derives it from duration_ms and distance_m when it is missing."),
		"watch_verified":   booleanSchema("Whether the watch verified the workout."),
		"splits": arraySchema("Optional partial laps or kilometres.", objectSchema(map[string]any{
			"position":        integerSchema("Zero-based split position."),
			"kind":            stringSchema("Optional split kind, such as km or lap."),
			"distance_m":      numberSchema("Optional split distance in metres."),
			"duration_ms":     integerSchema("Optional split duration in milliseconds."),
			"avg_hr_bpm":      integerSchema("Optional split average heart rate."),
			"pace_sec_per_km": numberSchema("Optional split pace. The server derives it from the split duration and distance when it is missing."),
		}, "position")),
	}, "origin", "external_id", "discipline")
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func booleanSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func arraySchema(description string, items map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": items}
}
