package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var errTrailingArguments = errors.New("arguments must contain a single JSON object")

func decodeStrict(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		trimmed = []byte("{}")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errTrailingArguments
		}
		return err
	}

	return nil
}

func validateArguments(schema map[string]any, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte("{}")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return paramsError{message: "invalid arguments: " + err.Error()}
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return paramsError{message: "invalid arguments: " + errTrailingArguments.Error()}
	}

	if err := validateValue(schema, value); err != nil {
		return paramsError{message: "invalid arguments: " + err.Error()}
	}
	return nil
}

func validateValue(schema map[string]any, value any) error {
	if value == nil {
		return nil
	}

	kind, _ := schema["type"].(string)
	switch kind {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}

		properties, _ := schema["properties"].(map[string]any)
		if properties == nil {
			properties = map[string]any{}
		}

		if required, ok := schema["required"].([]string); ok {
			for _, key := range required {
				if _, present := object[key]; !present {
					return fmt.Errorf("missing required argument %q", key)
				}
			}
		}

		strict, _ := schema["additionalProperties"].(bool)
		for key, item := range object {
			property, known := properties[key]
			if !known {
				if !strict {
					return fmt.Errorf("unknown argument %q", key)
				}
				continue
			}
			subschema, _ := property.(map[string]any)
			if subschema == nil {
				continue
			}
			if err := validateValue(subschema, item); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return errors.New("expected array")
		}
		itemSchema, _ := schema["items"].(map[string]any)
		if itemSchema == nil {
			return nil
		}
		for index, item := range items {
			if err := validateValue(itemSchema, item); err != nil {
				return fmt.Errorf("[%d]: %w", index, err)
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return errors.New("expected string")
		}
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("expected integer")
		}
		if _, err := number.Int64(); err != nil {
			return errors.New("expected integer")
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return errors.New("expected number")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.New("expected boolean")
		}
	}

	return nil
}

type listExercisesArgs struct {
	Cursor string `json:"cursor"`
}

type createExerciseArgs struct {
	IdempotencyKey  string                  `json:"idempotency_key"`
	Name            string                  `json:"name"`
	MovementPattern string                  `json:"movement_pattern"`
	Unilateral      bool                    `json:"unilateral"`
	Equipment       []exerciseEquipmentArgs `json:"equipment"`
}

type exerciseEquipmentArgs struct {
	EquipmentID string `json:"equipment_id"`
	Quantity    int    `json:"quantity"`
}

type createPlanArgs struct {
	IdempotencyKey  string            `json:"idempotency_key"`
	Reason          string            `json:"reason"`
	EffectiveFrom   string            `json:"effective_from"`
	Timezone        string            `json:"timezone"`
	WeeklyFrequency int               `json:"weekly_frequency"`
	GoalID          string            `json:"goal_id"`
	Sessions        []planSessionArgs `json:"sessions"`
}

type planSessionArgs struct {
	ID            string                `json:"id"`
	Position      int                   `json:"position"`
	ScheduledDate *string               `json:"scheduled_date"`
	Weekday       *int                  `json:"weekday"`
	Discipline    string                `json:"discipline"`
	Target        string                `json:"target"`
	Exercises     []sessionExerciseArgs `json:"exercises"`
	IntervalSteps []intervalStepArgs    `json:"interval_steps"`
}

type sessionExerciseArgs struct {
	Position   int      `json:"position"`
	ExerciseID string   `json:"exercise_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type intervalStepArgs struct {
	Position   int      `json:"position"`
	Label      string   `json:"label"`
	DurationMS *int     `json:"duration_ms"`
	DistanceM  *float64 `json:"distance_m"`
}

type getPlanArgs struct {
	ID int64 `json:"id"`
}

type weekArgs struct {
	Date string `json:"date"`
}

type getDayArgs struct {
	Date string `json:"date"`
}

type upsertDayArgs struct {
	IdempotencyKey   string         `json:"idempotency_key"`
	Date             string         `json:"date"`
	Timezone         string         `json:"timezone"`
	ExpectedRevision int64          `json:"expected_revision"`
	Source           string         `json:"source"`
	StepsSource      *string        `json:"steps_source"`
	Metrics          *metricsArgs   `json:"metrics"`
	SleepStages      *[]stageArgs   `json:"sleep_stages"`
	Workouts         *[]workoutArgs `json:"workouts"`
}

type metricsArgs struct {
	SleepMS      optionalInt64   `json:"sleep_ms"`
	Steps        optionalInt64   `json:"steps"`
	RestingHRBPM optionalInt64   `json:"resting_hr_bpm"`
	HRVRMSSDMS   optionalFloat64 `json:"hrv_rmssd_ms"`
}

type optionalInt64 struct {
	Set   bool
	Value *int64
}

func (o *optionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type optionalFloat64 struct {
	Set   bool
	Value *float64
}

func (o *optionalFloat64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type stageArgs struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}

type workoutArgs struct {
	Origin         string      `json:"origin"`
	ExternalID     string      `json:"external_id"`
	DeviceKind     *string     `json:"device_kind"`
	StartedAt      int64       `json:"started_at"`
	EndedAt        int64       `json:"ended_at"`
	Discipline     string      `json:"discipline"`
	DurationMS     int64       `json:"duration_ms"`
	DistanceM      *float64    `json:"distance_m"`
	AvgHRBPM       *int64      `json:"avg_hr_bpm"`
	MaxHRBPM       *int64      `json:"max_hr_bpm"`
	CaloriesKcal   *float64    `json:"calories_kcal"`
	CadenceSPM     *float64    `json:"cadence_spm"`
	ElevationGainM *float64    `json:"elevation_gain_m"`
	PaceSecPerKM   *float64    `json:"pace_sec_per_km"`
	WatchVerified  bool        `json:"watch_verified"`
	Splits         []splitArgs `json:"splits"`
}

type splitArgs struct {
	Position     int      `json:"position"`
	Kind         *string  `json:"kind"`
	DistanceM    *float64 `json:"distance_m"`
	DurationMS   *int64   `json:"duration_ms"`
	AvgHRBPM     *int64   `json:"avg_hr_bpm"`
	PaceSecPerKM *float64 `json:"pace_sec_per_km"`
}

type createSessionArgs struct {
	IdempotencyKey string  `json:"idempotency_key"`
	ID             string  `json:"id"`
	TemplateID     *string `json:"template_id"`
	Date           string  `json:"date"`
	Discipline     string  `json:"discipline"`
	StartedAt      *string `json:"started_at"`
}

type checkInSessionArgs struct {
	IdempotencyKey      string  `json:"idempotency_key"`
	SessionID           string  `json:"session_id"`
	ExpectedRevision    int64   `json:"expected_revision"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
}

type logSetsArgs struct {
	IdempotencyKey string    `json:"idempotency_key"`
	SessionID      string    `json:"session_id"`
	Sets           []setArgs `json:"sets"`
}

type setArgs struct {
	ExerciseID string   `json:"exercise_id"`
	SetIndex   int      `json:"set_index"`
	KG         *float64 `json:"kg"`
	Reps       int      `json:"reps"`
	Note       *string  `json:"note"`
}

type correctSetArgs struct {
	IdempotencyKey   string         `json:"idempotency_key"`
	SetID            string         `json:"set_id"`
	ExpectedRevision int64          `json:"expected_revision"`
	Reason           string         `json:"reason"`
	Changes          setChangesArgs `json:"changes"`
}

type setChangesArgs struct {
	ExerciseID *string  `json:"exercise_id"`
	SetIndex   *int     `json:"set_index"`
	KG         *float64 `json:"kg"`
	Reps       *int     `json:"reps"`
	Note       *string  `json:"note"`
}

type progressionArgs struct {
	ExerciseID string `json:"exercise_id"`
}
