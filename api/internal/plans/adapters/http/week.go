package http

import (
	"errors"
	nethttp "net/http"
	"sort"
	"time"

	"github.com/hectorruiz23/temper/api/internal/health/app"
	healthdomain "github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
	trainingdomain "github.com/hectorruiz23/temper/api/internal/training/domain"
)

var errInvalidDate = errors.New("invalid date")

type WeekHandler struct {
	plans    *plansapp.PlanService
	health   *app.Service
	training *trainingapp.Service
}

func NewWeekHandler(plans *plansapp.PlanService, health *app.Service, training *trainingapp.Service) *WeekHandler {
	return &WeekHandler{plans: plans, health: health, training: training}
}

func (h *WeekHandler) Get(w nethttp.ResponseWriter, request *nethttp.Request) {
	date, err := parseISODate(request.PathValue("date"))
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "date must be an ISO date")
		return
	}

	monday := mondayOf(date)
	weekStart := monday.Format("2006-01-02")
	weekEnd := monday.AddDate(0, 0, 6).Format("2006-01-02")

	plan, planErr := h.plans.PlanEffectiveOn(request.Context(), weekStart)
	if planErr != nil {
		var rule kernel.RuleError
		if !errors.As(planErr, &rule) || rule.Code != "not_found" {
			writeServiceError(w, planErr)
			return
		}
		plan = domain.Plan{}
	}

	location := time.UTC
	if plan.ID != 0 {
		location, err = time.LoadLocation(plan.Timezone)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	}

	planned := make([]plannedEntry, 0, 7)
	for offset := 0; offset < 7; offset++ {
		dayDate := dateInLocation(monday, location).AddDate(0, 0, offset).Format("2006-01-02")
		dayPlan, dayErr := h.plans.PlanEffectiveOn(request.Context(), dayDate)
		if dayErr != nil {
			var rule kernel.RuleError
			if !errors.As(dayErr, &rule) || rule.Code != "not_found" {
				writeServiceError(w, dayErr)
				return
			}
			continue
		}
		planned = append(planned, expandDay(dayPlan, dayDate, offset+1)...)
	}

	sessions, err := h.training.SessionsBetween(request.Context(), weekStart, weekEnd)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	performed := make([]performedEntry, 0, len(sessions))
	for _, session := range sessions {
		performed = append(performed, performedEntryFrom(session))
	}

	days := make([]weekDayResponse, 0, 7)
	for offset := 0; offset < 7; offset++ {
		dayDate := dateInLocation(monday, location).AddDate(0, 0, offset).Format("2006-01-02")
		day, err := h.health.GetDay(request.Context(), dayDate)
		if err != nil {
			if errors.Is(err, healthdomain.ErrDayNotFound) {
				continue
			}
			writeServiceError(w, err)
			return
		}
		days = append(days, weekDayResponse{
			Date:         day.Date,
			Revision:     day.Revision,
			SleepMS:      day.SleepMS,
			Steps:        day.Steps,
			RestingHRBPM: day.RestingHRBPM,
			HRVRMSSDMS:   day.HRVRMSSDMS,
			Workouts:     workoutsResponse(day.Workouts),
		})
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, weekResponse{
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
		Planned:   planned,
		Performed: performed,
		Days:      days,
	})
}

func parseISODate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, errInvalidDate
	}
	return parsed, nil
}

func mondayOf(date time.Time) time.Time {
	offset := (int(date.Weekday()) + 6) % 7
	return date.AddDate(0, 0, -offset)
}

func dateInLocation(date time.Time, location *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location)
}

func expandDay(plan domain.Plan, date string, weekday int) []plannedEntry {
	dated := make([]plannedCandidate, 0, len(plan.Sessions))
	recurring := make([]plannedCandidate, 0, len(plan.Sessions))

	for _, session := range plan.Sessions {
		entry := plannedEntry{
			Date:          date,
			TemplateID:    session.ID,
			Discipline:    session.Discipline,
			Target:        session.Target,
			PlanVersionID: plan.ID,
		}
		if session.ScheduledDate != nil {
			if *session.ScheduledDate == date {
				dated = append(dated, plannedCandidate{entry: entry, position: session.Position})
			}
			continue
		}
		if session.Weekday != nil && *session.Weekday == weekday {
			recurring = append(recurring, plannedCandidate{entry: entry, position: session.Position})
		}
	}

	sortCandidates(dated)
	sortCandidates(recurring)

	entries := make([]plannedEntry, 0, len(dated)+len(recurring))
	for _, candidate := range dated {
		entries = append(entries, candidate.entry)
	}
	for _, candidate := range recurring {
		entries = append(entries, candidate.entry)
	}
	return entries
}

func sortCandidates(candidates []plannedCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].position != candidates[j].position {
			return candidates[i].position < candidates[j].position
		}
		return candidates[i].entry.TemplateID < candidates[j].entry.TemplateID
	})
}

type plannedCandidate struct {
	entry    plannedEntry
	position int
}

type weekResponse struct {
	WeekStart string            `json:"week_start"`
	WeekEnd   string            `json:"week_end"`
	Planned   []plannedEntry    `json:"planned"`
	Performed []performedEntry  `json:"performed"`
	Days      []weekDayResponse `json:"days"`
}

type performedEntry struct {
	ID         string             `json:"id"`
	Date       string             `json:"date"`
	TemplateID *string            `json:"template_id"`
	Discipline string             `json:"discipline"`
	StartedAt  *string            `json:"started_at"`
	Revision   int64              `json:"revision"`
	Complete   bool               `json:"complete"`
	Evidence   *performedEvidence `json:"evidence"`
}

type performedEvidence struct {
	Source              string  `json:"source"`
	WorkoutID           *string `json:"workout_id,omitempty"`
	ReportedCompletedAt *string `json:"reported_completed_at,omitempty"`
}

func performedEntryFrom(session trainingdomain.Session) performedEntry {
	return performedEntry{
		ID:         session.ID,
		Date:       session.Date,
		TemplateID: session.TemplateID,
		Discipline: session.Discipline,
		StartedAt:  performedTimestamp(session.StartedAt),
		Revision:   session.Revision,
		Complete:   session.Complete,
		Evidence:   performedEvidenceFrom(trainingdomain.EffectiveEvidence(session, session.Complete)),
	}
}

func performedEvidenceFrom(evidence *trainingdomain.Evidence) *performedEvidence {
	if evidence == nil {
		return nil
	}
	return &performedEvidence{
		Source:              evidence.Source,
		WorkoutID:           evidence.WatchWorkoutID,
		ReportedCompletedAt: performedTimestamp(evidence.ReportedCompletedAt),
	}
}

func performedTimestamp(millis *int64) *string {
	if millis == nil {
		return nil
	}
	value := time.UnixMilli(*millis).UTC().Format(time.RFC3339Nano)
	return &value
}

type plannedEntry struct {
	Date          string `json:"date"`
	TemplateID    string `json:"template_id"`
	Discipline    string `json:"discipline"`
	Target        string `json:"target"`
	PlanVersionID int64  `json:"plan_version_id"`
}

type weekDayResponse struct {
	Date         string            `json:"date"`
	Revision     int64             `json:"revision"`
	SleepMS      *int64            `json:"sleep_ms"`
	Steps        *int64            `json:"steps"`
	RestingHRBPM *int64            `json:"resting_hr_bpm"`
	HRVRMSSDMS   *float64          `json:"hrv_rmssd_ms"`
	Workouts     []workoutResponse `json:"workouts"`
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

func workoutsResponse(workouts []healthdomain.Workout) []workoutResponse {
	present := make([]workoutResponse, 0, len(workouts))
	for _, workout := range workouts {
		if !workout.Present {
			continue
		}
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
		present = append(present, workoutResponse{
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
		})
	}
	return present
}
