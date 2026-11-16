package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (t *transaction) Set(ctx context.Context, id string) (domain.Set, error) {
	var (
		set            domain.Set
		note           sql.NullString
		idempotencyKey sql.NullString
	)
	err := t.query.QueryRowContext(ctx, `SELECT id, session_id, exercise_id, set_index, kg, reps, note, evidence_source, idempotency_key, created_at, revision
		FROM logged_set WHERE id = ?`, id).
		Scan(&set.ID, &set.SessionID, &set.ExerciseID, &set.SetIndex, &set.KG, &set.Reps, &note, &set.EvidenceSource, &idempotencyKey, &set.CreatedAt, &set.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Set{}, kernel.NewRuleError("not_found", "set not found")
	}
	if err != nil {
		return domain.Set{}, err
	}
	set.Note = nullableStringValue(note)
	set.IdempotencyKey = nullableStringValue(idempotencyKey)
	return set, nil
}

func (t *transaction) UpdateSet(ctx context.Context, set domain.Set, expectedRevision int64) error {
	result, err := t.query.ExecContext(ctx, `UPDATE logged_set
		SET exercise_id = ?, set_index = ?, kg = ?, reps = ?, note = ?, revision = ?
		WHERE id = ? AND revision = ?`,
		set.ExerciseID,
		set.SetIndex,
		set.KG,
		set.Reps,
		nullableString(set.Note),
		set.Revision,
		set.ID,
		expectedRevision,
	)
	if isUniqueViolation(err, "logged_set.session_id, logged_set.exercise_id, logged_set.set_index") {
		return kernel.NewRuleError("set_position_occupied", "a set already exists at this session, exercise and position")
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return kernel.NewRuleError("revision_conflict", "expected_revision does not match the stored revision")
	}
	return nil
}

func (t *transaction) InsertCorrection(ctx context.Context, correction domain.SetCorrection) error {
	_, err := t.query.ExecContext(ctx, `INSERT INTO set_correction (id, set_id, created_at, reason, before_json, after_json, resulting_revision, idempotency_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		correction.ID,
		correction.SetID,
		correction.CreatedAt,
		correction.Reason,
		correction.BeforeJSON,
		correction.AfterJSON,
		correction.ResultingRevision,
		nullableString(correction.IdempotencyKey),
	)
	if isUniqueViolation(err, "set_correction.set_id, set_correction.resulting_revision") {
		return kernel.NewRuleError("revision_conflict", "a correction already exists for this resulting revision")
	}
	return err
}

func (t *transaction) Equipment(ctx context.Context, exerciseID string) ([]domain.EquipmentRequirement, []domain.InventoryStock, error) {
	required, err := t.equipmentRequirements(ctx, exerciseID)
	if err != nil {
		return nil, nil, err
	}
	stock, err := t.inventoryStock(ctx, exerciseID)
	if err != nil {
		return nil, nil, err
	}
	return required, stock, nil
}

func (t *transaction) equipmentRequirements(ctx context.Context, exerciseID string) ([]domain.EquipmentRequirement, error) {
	rows, err := t.query.QueryContext(ctx, `SELECT equipment_id, quantity FROM exercise_equipment WHERE exercise_id = ? ORDER BY equipment_id`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	required := make([]domain.EquipmentRequirement, 0)
	for rows.Next() {
		var (
			requirement domain.EquipmentRequirement
			quantity    sql.NullInt64
		)
		if err := rows.Scan(&requirement.EquipmentID, &quantity); err != nil {
			return nil, err
		}
		requirement.Quantity = int(quantity.Int64)
		required = append(required, requirement)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return required, nil
}

func (t *transaction) inventoryStock(ctx context.Context, exerciseID string) ([]domain.InventoryStock, error) {
	rows, err := t.query.QueryContext(ctx, `SELECT ii.equipment_id, ii.quantity, ii.max_kg
		FROM inventory_item ii
		JOIN exercise_equipment ee ON ee.equipment_id = ii.equipment_id
		WHERE ee.exercise_id = ?
		ORDER BY ii.equipment_id`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stock := make([]domain.InventoryStock, 0)
	for rows.Next() {
		var (
			item     domain.InventoryStock
			quantity sql.NullInt64
			maxKG    sql.NullFloat64
		)
		if err := rows.Scan(&item.EquipmentID, &quantity, &maxKG); err != nil {
			return nil, err
		}
		item.Quantity = int(quantity.Int64)
		item.MaxKG = nullableFloatValue(maxKG)
		stock = append(stock, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stock, nil
}
