package domain

const (
	DisciplineStrength  = "strength"
	DisciplineRun       = "run"
	DisciplineIntervals = "intervals"
)

const (
	EvidenceWatch    = "watch"
	EvidenceReported = "reported"
)

type Session struct {
	ID                  string  `json:"id"`
	PlanVersionID       *int64  `json:"plan_version_id"`
	TemplateID          *string `json:"template_id"`
	Date                string  `json:"date"`
	Discipline          string  `json:"discipline"`
	StartedAt           *int64  `json:"started_at"`
	ReportedCompletedAt *int64  `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
	CreatedAt           int64   `json:"created_at"`
	Revision            int64   `json:"revision"`
	Complete            bool    `json:"complete"`
}

type NewSession struct {
	ID         string
	TemplateID *string
	Date       string
	Discipline string
	StartedAt  *int64
}

type NewSet struct {
	ExerciseID string  `json:"exercise_id"`
	SetIndex   int     `json:"set_index"`
	KG         float64 `json:"kg"`
	Reps       int     `json:"reps"`
	Note       *string `json:"note"`
}

type Set struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	ExerciseID     string  `json:"exercise_id"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	IdempotencyKey *string `json:"idempotency_key"`
	CreatedAt      int64   `json:"created_at"`
	Revision       int64   `json:"revision"`
}

type CheckInInput struct {
	ExpectedRevision    int64
	ReportedCompletedAt *int64
	WatchWorkoutID      *string
}

type Evidence struct {
	Source              string
	WatchWorkoutID      *string
	ReportedCompletedAt *int64
}

type Workout struct {
	ID            string
	Present       bool
	WatchVerified bool
}
