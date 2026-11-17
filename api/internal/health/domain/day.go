package domain

import "errors"

var ErrDayNotFound = errors.New("day not found")

const (
	SourceManual        = "manual"
	SourceHealthConnect = "health_connect"
)

type OptionalInt64 struct {
	Set   bool
	Value *int64
}

type OptionalFloat64 struct {
	Set   bool
	Value *float64
}

type MetricsPatch struct {
	SleepMS      OptionalInt64
	Steps        OptionalInt64
	RestingHRBPM OptionalInt64
	HRVRMSSDMS   OptionalFloat64
}

type SleepStage struct {
	Stage      string
	DurationMS int64
}

type Workout struct {
	ID             string
	Origin         string
	ExternalID     string
	DeviceKind     *string
	StartedAt      int64
	EndedAt        int64
	Discipline     string
	DurationMS     int64
	DistanceM      *float64
	AvgHRBPM       *int64
	MaxHRBPM       *int64
	CaloriesKcal   *float64
	CadenceSPM     *float64
	ElevationGainM *float64
	PaceSecPerKM   *float64
	WatchVerified  bool
	Present        bool
	Splits         []WorkoutSplit
}

type WorkoutSplit struct {
	Position     int
	Kind         *string
	DistanceM    *float64
	DurationMS   *int64
	AvgHRBPM     *int64
	PaceSecPerKM *float64
}

type Day struct {
	Date         string
	Timezone     string
	Revision     int64
	SleepMS      *int64
	Steps        *int64
	RestingHRBPM *int64
	HRVRMSSDMS   *float64
	UpdatedAt    int64
	LastSource   string
	SleepStages  []SleepStage
	Workouts     []Workout
}

type Upsert struct {
	Date             string
	Timezone         string
	ExpectedRevision int64
	Source           string
	Metrics          MetricsPatch
	SleepStages      *[]SleepStage
	Workouts         *[]Workout
}
