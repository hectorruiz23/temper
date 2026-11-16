package domain

const (
	DisciplineStrength  = "strength"
	DisciplineRun       = "run"
	DisciplineIntervals = "intervals"
)

type Plan struct {
	ID              int64     `json:"id"`
	CreatedAt       int64     `json:"created_at"`
	Reason          string    `json:"reason"`
	EffectiveFrom   string    `json:"effective_from"`
	Timezone        string    `json:"timezone"`
	WeeklyFrequency int       `json:"weekly_frequency"`
	GoalID          string    `json:"goal_id,omitempty"`
	Sessions        []Session `json:"sessions"`
}

type Session struct {
	ID            string            `json:"id"`
	Position      int               `json:"position"`
	ScheduledDate *string           `json:"scheduled_date,omitempty"`
	Weekday       *int              `json:"weekday,omitempty"`
	Discipline    string            `json:"discipline"`
	Target        string            `json:"target,omitempty"`
	Exercises     []SessionExercise `json:"exercises,omitempty"`
	Steps         []IntervalStep    `json:"steps,omitempty"`
}

type SessionExercise struct {
	Position   int      `json:"position"`
	ExerciseID string   `json:"exercise_id"`
	TargetSets *int     `json:"target_sets,omitempty"`
	RepsMin    *int     `json:"reps_min,omitempty"`
	RepsMax    *int     `json:"reps_max,omitempty"`
	TargetKG   *float64 `json:"target_kg,omitempty"`
	Tempo      *string  `json:"tempo,omitempty"`
}

type IntervalStep struct {
	Position   int      `json:"position"`
	Label      string   `json:"label"`
	DurationMS *int     `json:"duration_ms,omitempty"`
	DistanceM  *float64 `json:"distance_m,omitempty"`
}

type Exercise struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	MovementPattern string              `json:"movement_pattern"`
	Unilateral      bool                `json:"unilateral"`
	CreatedAt       int64               `json:"created_at"`
	Equipment       []ExerciseEquipment `json:"equipment"`
}

type ExerciseEquipment struct {
	EquipmentID string `json:"equipment_id"`
	Quantity    int    `json:"quantity"`
}

type InventoryItem struct {
	EquipmentID string   `json:"equipment_id"`
	Name        string   `json:"name"`
	Quantity    int      `json:"quantity"`
	MaxKG       *float64 `json:"max_kg,omitempty"`
}

type EquipmentRequirement struct {
	EquipmentID string
	Quantity    int
}

type InventoryStock struct {
	EquipmentID string
	Quantity    int
	MaxKG       *float64
}
