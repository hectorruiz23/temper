package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
)

func (s *Store) CreateExercise(ctx context.Context, exercise domain.Exercise) error {
	return s.run(ctx, func(ctx context.Context, query queryer) error {
		var count int
		if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM exercise WHERE id = ?`, exercise.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return kernel.NewRuleError("exercise_exists", fmt.Sprintf("exercise %q already exists", exercise.ID))
		}

		for _, equipment := range exercise.Equipment {
			if err := query.QueryRowContext(ctx, `SELECT COUNT(*) FROM equipment WHERE id = ?`, equipment.EquipmentID).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				return kernel.NewRuleError("unknown_equipment", fmt.Sprintf("unknown equipment %q", equipment.EquipmentID))
			}
		}

		if _, err := query.ExecContext(ctx, `INSERT INTO exercise (id, name, movement_pattern, unilateral, created_at) VALUES (?, ?, ?, ?, ?)`,
			exercise.ID, exercise.Name, exercise.MovementPattern, exercise.Unilateral, exercise.CreatedAt); err != nil {
			return err
		}

		for _, equipment := range exercise.Equipment {
			if _, err := query.ExecContext(ctx, `INSERT INTO exercise_equipment (exercise_id, equipment_id, quantity) VALUES (?, ?, ?)`,
				exercise.ID, equipment.EquipmentID, equipment.Quantity); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) ListExercises(ctx context.Context) ([]domain.Exercise, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT id, name, movement_pattern, unilateral, created_at FROM exercise ORDER BY id`)
	if err != nil {
		return nil, err
	}

	var exercises []domain.Exercise
	for rows.Next() {
		var (
			exercise   domain.Exercise
			unilateral sql.NullInt64
			createdAt  sql.NullInt64
		)
		if err := rows.Scan(&exercise.ID, &exercise.Name, &exercise.MovementPattern, &unilateral, &createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		exercise.Unilateral = unilateral.Int64 == 1
		exercise.CreatedAt = createdAt.Int64
		exercises = append(exercises, exercise)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for index := range exercises {
		equipment, err := s.loadExerciseEquipment(ctx, exercises[index].ID)
		if err != nil {
			return nil, err
		}
		exercises[index].Equipment = equipment
	}

	return exercises, nil
}

func (s *Store) loadExerciseEquipment(ctx context.Context, exerciseID string) ([]domain.ExerciseEquipment, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT equipment_id, quantity FROM exercise_equipment WHERE exercise_id = ? ORDER BY equipment_id`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	equipment := make([]domain.ExerciseEquipment, 0)
	for rows.Next() {
		var item domain.ExerciseEquipment
		if err := rows.Scan(&item.EquipmentID, &item.Quantity); err != nil {
			return nil, err
		}
		equipment = append(equipment, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return equipment, nil
}

func (s *Store) Inventory(ctx context.Context) ([]domain.InventoryItem, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT e.id, e.name, i.quantity, i.max_kg FROM inventory_item i JOIN equipment e ON e.id = i.equipment_id ORDER BY e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.InventoryItem, 0)
	for rows.Next() {
		var (
			item     domain.InventoryItem
			quantity sql.NullInt64
			maxKG    sql.NullFloat64
		)
		if err := rows.Scan(&item.EquipmentID, &item.Name, &quantity, &maxKG); err != nil {
			return nil, err
		}
		item.Quantity = int(quantity.Int64)
		if maxKG.Valid {
			value := maxKG.Float64
			item.MaxKG = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
