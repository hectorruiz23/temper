package acceptance

import (
	"context"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/plans/app"
)

func TestPlanVersionReusesSessionIDs(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)

	first := acceptancePlan("goblet-squat", "shared")
	created, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{IdempotencyKey: "shared-v1", Plan: first})
	if err != nil {
		t.Fatalf("CreatePlan first version: %v", err)
	}

	second := acceptancePlan("goblet-squat", "shared")
	second.EffectiveFrom = "2027-01-01"
	second.Reason = "second block, same sessions"
	second.Sessions[0].Target = "full body, heavier"
	second.Sessions[0].Exercises[0].TargetKG = planFloatPtr(16)
	updated, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{IdempotencyKey: "shared-v2", Plan: second})
	if err != nil {
		t.Fatalf("CreatePlan second version reusing session ids: %v", err)
	}
	if updated.ID == created.ID {
		t.Fatalf("second version reused plan id %d", created.ID)
	}

	older, err := fixture.plans.GetPlan(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlan first version: %v", err)
	}
	if len(older.Sessions) != len(second.Sessions) {
		t.Fatalf("first version sessions = %d, want %d", len(older.Sessions), len(second.Sessions))
	}
	for index, session := range older.Sessions {
		if session.ID != second.Sessions[index].ID {
			t.Fatalf("first version session %d = %q, want %q", index, session.ID, second.Sessions[index].ID)
		}
		if session.Target != first.Sessions[index].Target {
			t.Fatalf("first version session %q target = %q, want %q", session.ID, session.Target, first.Sessions[index].Target)
		}
	}

	newer, err := fixture.plans.GetPlan(ctx, updated.ID)
	if err != nil {
		t.Fatalf("GetPlan second version: %v", err)
	}
	if newer.Sessions[0].Target != "full body, heavier" {
		t.Fatalf("second version session target = %q", newer.Sessions[0].Target)
	}
	if len(newer.Sessions[0].Exercises) != 1 || newer.Sessions[0].Exercises[0].TargetKG == nil || *newer.Sessions[0].Exercises[0].TargetKG != 16 {
		t.Fatalf("second version exercises = %+v", newer.Sessions[0].Exercises)
	}

	effective, err := fixture.plans.PlanEffectiveOn(ctx, "2027-02-01")
	if err != nil {
		t.Fatalf("PlanEffectiveOn: %v", err)
	}
	if effective.ID != updated.ID {
		t.Fatalf("effective version = %d, want %d", effective.ID, updated.ID)
	}
	if len(effective.Sessions[1].Steps) != 2 {
		t.Fatalf("second version interval steps = %d, want 2", len(effective.Sessions[1].Steps))
	}
	if len(effective.Sessions[0].Exercises) != 1 {
		t.Fatalf("second version strength exercises = %d, want 1", len(effective.Sessions[0].Exercises))
	}
}

func TestPlanVersionRejectsDuplicateSessionID(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)

	plan := acceptancePlan("goblet-squat", "twice")
	duplicate := plan.Sessions[0]
	duplicate.Target = "same id, other position"
	plan.Sessions = append(plan.Sessions, duplicate)

	_, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{IdempotencyKey: "twice", Plan: plan})
	requirePlanCode(t, err, "invalid_input")
	if count := planCount(t, fixture.database); count != 0 {
		t.Fatalf("plan_version rows = %d after rejection, want 0", count)
	}
}
