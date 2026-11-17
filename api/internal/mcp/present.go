package mcp

import (
	"time"

	healthdomain "github.com/hectorruiz23/temper/api/internal/health/domain"
	plansdomain "github.com/hectorruiz23/temper/api/internal/plans/domain"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
	trainingdomain "github.com/hectorruiz23/temper/api/internal/training/domain"
)

type inventoryResponse struct {
	Items []inventoryItemResponse `json:"items"`
}

type inventoryItemResponse struct {
	EquipmentID string   `json:"equipment_id"`
	Name        string   `json:"name"`
	Quantity    int      `json:"quantity"`
	MaxKG       *float64 `json:"max_kg"`
}

func inventoryResponseFrom(items []plansdomain.InventoryItem) inventoryResponse {
	response := inventoryResponse{Items: make([]inventoryItemResponse, len(items))}
	for index, item := range items {
		response.Items[index] = inventoryItemResponse{
			EquipmentID: item.EquipmentID,
			Name:        item.Name,
			Quantity:    item.Quantity,
			MaxKG:       item.MaxKG,
		}
	}
	return response
}

type exerciseEnvelope struct {
	Exercise plansdomain.Exercise `json:"exercise"`
}

type exerciseListResponse struct {
	Exercises  []plansdomain.Exercise `json:"exercises"`
	NextCursor *string                `json:"next_cursor,omitempty"`
}

type planEnvelope struct {
	Plan planResponse `json:"plan"`
}

type planResponse struct {
	ID              int64                 `json:"id"`
	CreatedAt       string                `json:"created_at"`
	Reason          string                `json:"reason"`
	EffectiveFrom   string                `json:"effective_from"`
	Timezone        string                `json:"timezone"`
	WeeklyFrequency int                   `json:"weekly_frequency"`
	GoalID          string                `json:"goal_id"`
	Sessions        []planSessionResponse `json:"sessions"`
}

type planSessionResponse struct {
	ID            string                        `json:"id"`
	Position      int                           `json:"position"`
	ScheduledDate *string                       `json:"scheduled_date"`
	Weekday       *int                          `json:"weekday"`
	Discipline    string                        `json:"discipline"`
	Target        string                        `json:"target"`
	Exercises     []planSessionExerciseResponse `json:"exercises"`
	IntervalSteps []planIntervalStepResponse    `json:"interval_steps"`
}

type planSessionExerciseResponse struct {
	Position   int      `json:"position"`
	ExerciseID string   `json:"exercise_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type planIntervalStepResponse struct {
	Position   int      `json:"position"`
	Label      string   `json:"label"`
	DurationMS *int     `json:"duration_ms"`
	DistanceM  *float64 `json:"distance_m"`
}

func planResponseFrom(plan plansdomain.Plan) planResponse {
	sessions := make([]planSessionResponse, len(plan.Sessions))
	for index, session := range plan.Sessions {
		exercises := make([]planSessionExerciseResponse, len(session.Exercises))
		for exerciseIndex, exercise := range session.Exercises {
			exercises[exerciseIndex] = planSessionExerciseResponse{
				Position:   exercise.Position,
				ExerciseID: exercise.ExerciseID,
				TargetSets: exercise.TargetSets,
				RepsMin:    exercise.RepsMin,
				RepsMax:    exercise.RepsMax,
				TargetKG:   exercise.TargetKG,
				Tempo:      exercise.Tempo,
			}
		}

		steps := make([]planIntervalStepResponse, len(session.Steps))
		for stepIndex, step := range session.Steps {
			steps[stepIndex] = planIntervalStepResponse{
				Position:   step.Position,
				Label:      step.Label,
				DurationMS: step.DurationMS,
				DistanceM:  step.DistanceM,
			}
		}

		sessions[index] = planSessionResponse{
			ID:            session.ID,
			Position:      session.Position,
			ScheduledDate: session.ScheduledDate,
			Weekday:       session.Weekday,
			Discipline:    session.Discipline,
			Target:        session.Target,
			Exercises:     exercises,
			IntervalSteps: steps,
		}
	}

	return planResponse{
		ID:              plan.ID,
		CreatedAt:       time.UnixMilli(plan.CreatedAt).UTC().Format(time.RFC3339),
		Reason:          plan.Reason,
		EffectiveFrom:   plan.EffectiveFrom,
		Timezone:        plan.Timezone,
		WeeklyFrequency: plan.WeeklyFrequency,
		GoalID:          plan.GoalID,
		Sessions:        sessions,
	}
}

type dayEnvelope struct {
	Day dayResponse `json:"day"`
}

type dayResponse struct {
	Date         string            `json:"date"`
	Timezone     string            `json:"timezone"`
	Revision     int64             `json:"revision"`
	SleepMS      *int64            `json:"sleep_ms"`
	Steps        *int64            `json:"steps"`
	RestingHRBPM *int64            `json:"resting_hr_bpm"`
	HRVRMSSDMS   *float64          `json:"hrv_rmssd_ms"`
	UpdatedAt    int64             `json:"updated_at"`
	LastSource   string            `json:"last_source"`
	SleepStages  []stageResponse   `json:"sleep_stages"`
	Workouts     []workoutResponse `json:"workouts"`
}

type stageResponse struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}

type workoutResponse struct {
	ID             string          `json:"id"`
	Origin         string          `json:"origin"`
	ExternalID     string          `json:"external_id"`
	DeviceKind     *string         `json:"device_kind"`
	StartedAt      int64           `json:"started_at"`
	EndedAt        int64           `json:"ended_at"`
	Discipline     string          `json:"discipline"`
	DurationMS     int64           `json:"duration_ms"`
	DistanceM      *float64        `json:"distance_m"`
	AvgHRBPM       *int64          `json:"avg_hr_bpm"`
	MaxHRBPM       *int64          `json:"max_hr_bpm"`
	CaloriesKcal   *float64        `json:"calories_kcal"`
	CadenceSPM     *float64        `json:"cadence_spm"`
	ElevationGainM *float64        `json:"elevation_gain_m"`
	PaceSecPerKM   *float64        `json:"pace_sec_per_km"`
	WatchVerified  bool            `json:"watch_verified"`
	Splits         []splitResponse `json:"splits"`
}

type splitResponse struct {
	Position     int      `json:"position"`
	Kind         *string  `json:"kind"`
	DistanceM    *float64 `json:"distance_m"`
	DurationMS   *int64   `json:"duration_ms"`
	AvgHRBPM     *int64   `json:"avg_hr_bpm"`
	PaceSecPerKM *float64 `json:"pace_sec_per_km"`
}

func workoutResponseFrom(workout healthdomain.Workout) workoutResponse {
	splits := make([]splitResponse, 0, len(workout.Splits))
	for _, split := range workout.Splits {
		splits = append(splits, splitResponse{
			Position:     split.Position,
			Kind:         split.Kind,
			DistanceM:    split.DistanceM,
			DurationMS:   split.DurationMS,
			AvgHRBPM:     split.AvgHRBPM,
			PaceSecPerKM: split.PaceSecPerKM,
		})
	}
	return workoutResponse{
		ID:             workout.ID,
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
		Splits:         splits,
	}
}

func dayResponseFrom(day healthdomain.Day) dayResponse {
	stages := make([]stageResponse, len(day.SleepStages))
	for index, stage := range day.SleepStages {
		stages[index] = stageResponse{Stage: stage.Stage, DurationMS: stage.DurationMS}
	}

	workouts := make([]workoutResponse, 0, len(day.Workouts))
	for _, workout := range day.Workouts {
		if !workout.Present {
			continue
		}
		workouts = append(workouts, workoutResponseFrom(workout))
	}

	return dayResponse{
		Date:         day.Date,
		Timezone:     day.Timezone,
		Revision:     day.Revision,
		SleepMS:      day.SleepMS,
		Steps:        day.Steps,
		RestingHRBPM: day.RestingHRBPM,
		HRVRMSSDMS:   day.HRVRMSSDMS,
		UpdatedAt:    day.UpdatedAt,
		LastSource:   day.LastSource,
		SleepStages:  stages,
		Workouts:     workouts,
	}
}

type sessionEnvelope struct {
	Session sessionResponse `json:"session"`
}

type checkInEnvelope struct {
	Session           sessionResponse   `json:"session"`
	EffectiveEvidence *evidenceResponse `json:"effective_evidence"`
}

type sessionResponse struct {
	ID                  string  `json:"id"`
	TemplateID          *string `json:"template_id"`
	Date                string  `json:"date"`
	Discipline          string  `json:"discipline"`
	StartedAt           *string `json:"started_at"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
	CreatedAt           string  `json:"created_at"`
	Revision            int64   `json:"revision"`
	Complete            bool    `json:"complete"`
}

type evidenceResponse struct {
	Source              string  `json:"source"`
	WorkoutID           *string `json:"workout_id,omitempty"`
	ReportedCompletedAt *string `json:"reported_completed_at,omitempty"`
}

func sessionResponseFrom(session trainingdomain.Session) sessionResponse {
	return sessionResponse{
		ID:                  session.ID,
		TemplateID:          session.TemplateID,
		Date:                session.Date,
		Discipline:          session.Discipline,
		StartedAt:           nullableTimestamp(session.StartedAt),
		ReportedCompletedAt: nullableTimestamp(session.ReportedCompletedAt),
		WatchWorkoutID:      session.WatchWorkoutID,
		CreatedAt:           timestamp(session.CreatedAt),
		Revision:            session.Revision,
		Complete:            session.Complete,
	}
}

func evidenceResponseFrom(evidence *trainingdomain.Evidence) *evidenceResponse {
	if evidence == nil {
		return nil
	}
	return &evidenceResponse{
		Source:              evidence.Source,
		WorkoutID:           evidence.WatchWorkoutID,
		ReportedCompletedAt: nullableTimestamp(evidence.ReportedCompletedAt),
	}
}

type batchResponse struct {
	Sets []setResponse     `json:"sets"`
	Echo batchEchoResponse `json:"echo"`
}

type batchEchoResponse struct {
	SessionID string            `json:"session_id"`
	Sets      []echoSetResponse `json:"sets"`
}

type echoSetResponse struct {
	ExerciseID string  `json:"exercise_id"`
	SetIndex   int     `json:"set_index"`
	KG         float64 `json:"kg"`
	Reps       int     `json:"reps"`
	Note       *string `json:"note"`
}

type setResponse struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	ExerciseID     string  `json:"exercise_id"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	CreatedAt      string  `json:"created_at"`
	Revision       int64   `json:"revision"`
}

func setResponseFrom(set trainingdomain.Set) setResponse {
	return setResponse{
		ID:             set.ID,
		SessionID:      set.SessionID,
		ExerciseID:     set.ExerciseID,
		SetIndex:       set.SetIndex,
		KG:             set.KG,
		Reps:           set.Reps,
		Note:           set.Note,
		EvidenceSource: set.EvidenceSource,
		CreatedAt:      timestamp(set.CreatedAt),
		Revision:       set.Revision,
	}
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

func correctionResponseFrom(result trainingapp.CorrectionResult) correctionResponse {
	return correctionResponse{
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
	}
}

func timestamp(millis int64) string {
	return time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
}

func nullableTimestamp(millis *int64) *string {
	if millis == nil {
		return nil
	}
	value := timestamp(*millis)
	return &value
}
