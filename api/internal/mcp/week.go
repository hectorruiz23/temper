package mcp

import (
	"context"
	"errors"
	"sort"
	"time"

	healthdomain "github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	plansdomain "github.com/hectorruiz23/temper/api/internal/plans/domain"
	trainingdomain "github.com/hectorruiz23/temper/api/internal/training/domain"
)

type weekResponse struct {
	WeekStart string            `json:"week_start"`
	WeekEnd   string            `json:"week_end"`
	Planned   []plannedEntry    `json:"planned"`
	Performed []performedEntry  `json:"performed"`
	Days      []weekDayResponse `json:"days"`
}

type plannedEntry struct {
	Date          string `json:"date"`
	TemplateID    string `json:"template_id"`
	Discipline    string `json:"discipline"`
	Target        string `json:"target"`
	PlanVersionID int64  `json:"plan_version_id"`
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

type weekDayResponse struct {
	Date         string   `json:"date"`
	Revision     int64    `json:"revision"`
	SleepMS      *int64   `json:"sleep_ms"`
	Steps        *int64   `json:"steps"`
	RestingHRBPM *int64   `json:"resting_hr_bpm"`
	HRVRMSSDMS   *float64 `json:"hrv_rmssd_ms"`
}

type plannedCandidate struct {
	entry    plannedEntry
	position int
}

func (h *Handler) week(ctx context.Context, date string) (weekResponse, error) {
	parsed, err := parseISODate(date)
	if err != nil {
		return weekResponse{}, kernel.NewRuleError("invalid_input", "date must be an ISO date")
	}

	monday := mondayOf(parsed)
	weekStart := monday.Format("2006-01-02")
	weekEnd := monday.AddDate(0, 0, 6).Format("2006-01-02")

	plan, planErr := h.plans.PlanEffectiveOn(ctx, weekStart)
	if planErr != nil {
		var rule kernel.RuleError
		if !errors.As(planErr, &rule) || rule.Code != "not_found" {
			return weekResponse{}, planErr
		}
		plan = plansdomain.Plan{}
	}

	location := time.UTC
	if plan.ID != 0 {
		location, err = time.LoadLocation(plan.Timezone)
		if err != nil {
			return weekResponse{}, err
		}
	}

	planned := make([]plannedEntry, 0, 7)
	for offset := 0; offset < 7; offset++ {
		dayDate := dateInLocation(monday, location).AddDate(0, 0, offset).Format("2006-01-02")
		dayPlan, dayErr := h.plans.PlanEffectiveOn(ctx, dayDate)
		if dayErr != nil {
			var rule kernel.RuleError
			if !errors.As(dayErr, &rule) || rule.Code != "not_found" {
				return weekResponse{}, dayErr
			}
			continue
		}
		planned = append(planned, expandDay(dayPlan, dayDate, offset+1)...)
	}

	days := make([]weekDayResponse, 0, 7)
	for offset := 0; offset < 7; offset++ {
		dayDate := dateInLocation(monday, location).AddDate(0, 0, offset).Format("2006-01-02")
		day, err := h.health.GetDay(ctx, dayDate)
		if err != nil {
			if errors.Is(err, healthdomain.ErrDayNotFound) {
				continue
			}
			return weekResponse{}, err
		}
		days = append(days, weekDayResponse{
			Date:         day.Date,
			Revision:     day.Revision,
			SleepMS:      day.SleepMS,
			Steps:        day.Steps,
			RestingHRBPM: day.RestingHRBPM,
			HRVRMSSDMS:   day.HRVRMSSDMS,
		})
	}

	sessions, err := h.training.SessionsBetween(ctx, weekStart, weekEnd)
	if err != nil {
		return weekResponse{}, err
	}
	performed := make([]performedEntry, 0, len(sessions))
	for _, session := range sessions {
		performed = append(performed, performedEntryFrom(session))
	}

	return weekResponse{
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
		Planned:   planned,
		Performed: performed,
		Days:      days,
	}, nil
}

func parseISODate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, errors.New("invalid date")
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

func expandDay(plan plansdomain.Plan, date string, weekday int) []plannedEntry {
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
