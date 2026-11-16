package domain

import (
	"crypto/rand"
	"encoding/hex"
)

func ReconcileWorkouts(existing, incoming []Workout) ([]Workout, error) {
	if err := validateWorkouts(incoming); err != nil {
		return nil, err
	}

	previousByKey := make(map[string]Workout, len(existing))
	for _, workout := range existing {
		previousByKey[workoutKey(workout)] = workout
	}

	incomingKeys := make(map[string]bool, len(incoming))
	next := make([]Workout, 0, len(incoming)+len(existing))

	for _, workout := range incoming {
		key := workoutKey(workout)
		incomingKeys[key] = true
		workout.Present = true
		if previous, ok := previousByKey[key]; ok {
			workout.ID = previous.ID
		} else {
			id, err := newWorkoutID()
			if err != nil {
				return nil, err
			}
			workout.ID = id
		}
		next = append(next, workout)
	}

	for _, previous := range existing {
		if incomingKeys[workoutKey(previous)] {
			continue
		}
		previous.Present = false
		next = append(next, previous)
	}

	return next, nil
}

func workoutKey(workout Workout) string {
	return workout.Origin + "\x00" + workout.ExternalID
}

func newWorkoutID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
