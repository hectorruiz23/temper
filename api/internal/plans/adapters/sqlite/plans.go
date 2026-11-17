package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
)

func (s *Store) CreatePlan(ctx context.Context, plan domain.Plan) (int64, error) {
	var id int64

	err := s.run(ctx, func(ctx context.Context, query queryer) error {
		if err := checkGoal(ctx, query, plan.GoalID); err != nil {
			return err
		}
		if err := checkPlanExercises(ctx, query, plan); err != nil {
			return err
		}

		result, err := query.ExecContext(ctx, `INSERT INTO plan_version (created_at, reason, effective_from, timezone, weekly_frequency, goal_id) VALUES (?, ?, ?, ?, ?, ?)`,
			plan.CreatedAt, plan.Reason, plan.EffectiveFrom, plan.Timezone, plan.WeeklyFrequency, nullString(plan.GoalID))
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}

		for _, session := range plan.Sessions {
			if _, err := query.ExecContext(ctx, `INSERT INTO session_template (id, plan_version_id, position, scheduled_date, weekday, discipline, target) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				session.ID, id, session.Position, nullableString(session.ScheduledDate), nullableInt(session.Weekday), session.Discipline, nullString(session.Target)); err != nil {
				return duplicateTemplate(session, err)
			}
			for _, exercise := range session.Exercises {
				if _, err := query.ExecContext(ctx, `INSERT INTO session_exercise (plan_version_id, template_id, position, exercise_id, target_sets, reps_min, reps_max, target_kg, tempo) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, session.ID, exercise.Position, exercise.ExerciseID, nullableInt(exercise.TargetSets), nullableInt(exercise.RepsMin), nullableInt(exercise.RepsMax), nullableFloat(exercise.TargetKG), nullableString(exercise.Tempo)); err != nil {
					return err
				}
			}
			for _, step := range session.Steps {
				if _, err := query.ExecContext(ctx, `INSERT INTO interval_step (plan_version_id, template_id, position, label, duration_ms, distance_m) VALUES (?, ?, ?, ?, ?, ?)`,
					id, session.ID, step.Position, step.Label, nullableInt(step.DurationMS), nullableFloat(step.DistanceM)); err != nil {
					return err
				}
			}
		}

		return nil
	})

	return id, err
}

func (s *Store) CurrentPlan(ctx context.Context) (domain.Plan, error) {
	var id int64
	err := s.executor(ctx).QueryRowContext(ctx, `SELECT id FROM plan_version ORDER BY effective_from DESC, id DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Plan{}, kernel.NewRuleError("not_found", "no plan version exists")
	}
	if err != nil {
		return domain.Plan{}, err
	}
	return s.loadPlan(ctx, id)
}

func (s *Store) Plan(ctx context.Context, id int64) (domain.Plan, error) {
	return s.loadPlan(ctx, id)
}

func (s *Store) PlanEffectiveOn(ctx context.Context, date string) (domain.Plan, error) {
	var id int64
	err := s.executor(ctx).QueryRowContext(ctx, `SELECT id FROM plan_version WHERE effective_from <= ? ORDER BY effective_from DESC, id DESC LIMIT 1`, date).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Plan{}, kernel.NewRuleError("not_found", fmt.Sprintf("no plan version is effective on %s", date))
	}
	if err != nil {
		return domain.Plan{}, err
	}
	return s.loadPlan(ctx, id)
}

func duplicateTemplate(session domain.Session, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if strings.Contains(message, "UNIQUE constraint failed: session_template.plan_version_id, session_template.id") {
		return kernel.NewRuleError("session_id_taken", fmt.Sprintf("session id %q is already used by another session in this plan version", session.ID))
	}
	if strings.Contains(message, "UNIQUE constraint failed: session_template.plan_version_id, session_template.position") {
		return kernel.NewRuleError("session_position_taken", fmt.Sprintf("position %d is already used by another session in this plan version", session.Position))
	}
	return err
}

func checkGoal(ctx context.Context, query queryer, goalID string) error {
	if goalID == "" {
		return nil
	}
	var count int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM training_goal WHERE id = ?`, goalID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return kernel.NewRuleError("invalid_input", fmt.Sprintf("unknown training goal %q", goalID))
	}
	return nil
}

type equipmentFacts struct {
	required []domain.EquipmentRequirement
	stock    []domain.InventoryStock
}

func checkPlanExercises(ctx context.Context, query queryer, plan domain.Plan) error {
	known := make(map[string]equipmentFacts)
	for _, session := range plan.Sessions {
		for _, exercise := range session.Exercises {
			facts, ok := known[exercise.ExerciseID]
			if !ok {
				var count int
				if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM exercise WHERE id = ?`, exercise.ExerciseID).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					return kernel.NewRuleError("unknown_exercise", fmt.Sprintf("exercise %q does not exist", exercise.ExerciseID))
				}

				required, stock, err := exerciseEquipment(ctx, query, exercise.ExerciseID)
				if err != nil {
					return err
				}
				facts = equipmentFacts{required: required, stock: stock}
				known[exercise.ExerciseID] = facts
			}

			if err := domain.CheckEquipment(exercise.ExerciseID, facts.required, facts.stock, exercise.TargetKG); err != nil {
				return err
			}
		}
	}
	return nil
}

func exerciseEquipment(ctx context.Context, query queryer, exerciseID string) ([]domain.EquipmentRequirement, []domain.InventoryStock, error) {
	rows, err := query.QueryContext(ctx, `SELECT ee.equipment_id, ee.quantity, ii.quantity, ii.max_kg
		FROM exercise_equipment ee
		LEFT JOIN inventory_item ii ON ii.equipment_id = ee.equipment_id
		WHERE ee.exercise_id = ?
		ORDER BY ee.equipment_id`, exerciseID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var required []domain.EquipmentRequirement
	var stock []domain.InventoryStock
	for rows.Next() {
		var equipmentID string
		var quantity int
		var stockQuantity sql.NullInt64
		var maxKG sql.NullFloat64
		if err := rows.Scan(&equipmentID, &quantity, &stockQuantity, &maxKG); err != nil {
			return nil, nil, err
		}
		required = append(required, domain.EquipmentRequirement{EquipmentID: equipmentID, Quantity: quantity})
		if stockQuantity.Valid {
			item := domain.InventoryStock{EquipmentID: equipmentID, Quantity: int(stockQuantity.Int64)}
			if maxKG.Valid {
				value := maxKG.Float64
				item.MaxKG = &value
			}
			stock = append(stock, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return required, stock, nil
}

func (s *Store) loadPlan(ctx context.Context, id int64) (domain.Plan, error) {
	var (
		plan          domain.Plan
		createdAt     sql.NullInt64
		effectiveFrom sql.NullString
		timezone      sql.NullString
		weekly        sql.NullInt64
		goalID        sql.NullString
	)
	err := s.executor(ctx).QueryRowContext(ctx, `SELECT id, created_at, reason, effective_from, timezone, weekly_frequency, goal_id FROM plan_version WHERE id = ?`, id).
		Scan(&plan.ID, &createdAt, &plan.Reason, &effectiveFrom, &timezone, &weekly, &goalID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Plan{}, kernel.NewRuleError("not_found", fmt.Sprintf("plan version %d not found", id))
	}
	if err != nil {
		return domain.Plan{}, err
	}

	plan.CreatedAt = createdAt.Int64
	plan.EffectiveFrom = effectiveFrom.String
	plan.Timezone = timezone.String
	plan.WeeklyFrequency = int(weekly.Int64)
	plan.GoalID = goalID.String

	sessions, err := s.loadSessions(ctx, s.executor(ctx), id)
	if err != nil {
		return domain.Plan{}, err
	}
	plan.Sessions = sessions

	return plan, nil
}

func (s *Store) loadSessions(ctx context.Context, query queryer, planID int64) ([]domain.Session, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, position, scheduled_date, weekday, discipline, target FROM session_template WHERE plan_version_id = ? ORDER BY position, id`, planID)
	if err != nil {
		return nil, err
	}

	var sessions []domain.Session
	for rows.Next() {
		var (
			session       domain.Session
			position      sql.NullInt64
			scheduledDate sql.NullString
			weekday       sql.NullInt64
			discipline    sql.NullString
			target        sql.NullString
		)
		if err := rows.Scan(&session.ID, &position, &scheduledDate, &weekday, &discipline, &target); err != nil {
			rows.Close()
			return nil, err
		}
		session.Position = int(position.Int64)
		if scheduledDate.Valid {
			value := scheduledDate.String
			session.ScheduledDate = &value
		}
		if weekday.Valid {
			value := int(weekday.Int64)
			session.Weekday = &value
		}
		session.Discipline = discipline.String
		session.Target = target.String
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for index := range sessions {
		exercises, err := s.loadSessionExercises(ctx, query, planID, sessions[index].ID)
		if err != nil {
			return nil, err
		}
		sessions[index].Exercises = exercises

		steps, err := s.loadIntervalSteps(ctx, query, planID, sessions[index].ID)
		if err != nil {
			return nil, err
		}
		sessions[index].Steps = steps
	}

	return sessions, nil
}

func (s *Store) loadSessionExercises(ctx context.Context, query queryer, planID int64, templateID string) ([]domain.SessionExercise, error) {
	rows, err := query.QueryContext(ctx, `SELECT position, exercise_id, target_sets, reps_min, reps_max, target_kg, tempo FROM session_exercise WHERE plan_version_id = ? AND template_id = ? ORDER BY position`, planID, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exercises []domain.SessionExercise
	for rows.Next() {
		var (
			exercise   domain.SessionExercise
			position   sql.NullInt64
			targetSets sql.NullInt64
			repsMin    sql.NullInt64
			repsMax    sql.NullInt64
			targetKG   sql.NullFloat64
			tempo      sql.NullString
		)
		if err := rows.Scan(&position, &exercise.ExerciseID, &targetSets, &repsMin, &repsMax, &targetKG, &tempo); err != nil {
			return nil, err
		}
		exercise.Position = int(position.Int64)
		if targetSets.Valid {
			value := int(targetSets.Int64)
			exercise.TargetSets = &value
		}
		if repsMin.Valid {
			value := int(repsMin.Int64)
			exercise.RepsMin = &value
		}
		if repsMax.Valid {
			value := int(repsMax.Int64)
			exercise.RepsMax = &value
		}
		if targetKG.Valid {
			value := targetKG.Float64
			exercise.TargetKG = &value
		}
		if tempo.Valid {
			value := tempo.String
			exercise.Tempo = &value
		}
		exercises = append(exercises, exercise)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return exercises, nil
}

func (s *Store) loadIntervalSteps(ctx context.Context, query queryer, planID int64, templateID string) ([]domain.IntervalStep, error) {
	rows, err := query.QueryContext(ctx, `SELECT position, label, duration_ms, distance_m FROM interval_step WHERE plan_version_id = ? AND template_id = ? ORDER BY position`, planID, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []domain.IntervalStep
	for rows.Next() {
		var (
			step       domain.IntervalStep
			position   sql.NullInt64
			durationMS sql.NullInt64
			distanceM  sql.NullFloat64
		)
		if err := rows.Scan(&position, &step.Label, &durationMS, &distanceM); err != nil {
			return nil, err
		}
		step.Position = int(position.Int64)
		if durationMS.Valid {
			value := int(durationMS.Int64)
			step.DurationMS = &value
		}
		if distanceM.Valid {
			value := distanceM.Float64
			step.DistanceM = &value
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return steps, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
