-- A workout kept the duration and the distance and threw away everything else
-- the watch reported. The remaining aggregates live on day_workout, and the
-- partial laps go to their own table.
--
-- Every column is nullable on purpose: a workout that came from a manual note
-- has no heart rate, and a missing value is not a zero.
ALTER TABLE day_workout ADD COLUMN avg_hr_bpm INTEGER NULL CHECK (avg_hr_bpm > 0);
ALTER TABLE day_workout ADD COLUMN max_hr_bpm INTEGER NULL CHECK (max_hr_bpm > 0);
ALTER TABLE day_workout ADD COLUMN calories_kcal REAL NULL CHECK (calories_kcal >= 0);
ALTER TABLE day_workout ADD COLUMN cadence_spm REAL NULL CHECK (cadence_spm >= 0);
ALTER TABLE day_workout ADD COLUMN elevation_gain_m REAL NULL CHECK (elevation_gain_m >= 0);
ALTER TABLE day_workout ADD COLUMN pace_sec_per_km REAL NULL CHECK (pace_sec_per_km > 0);

CREATE TABLE workout_split (
    workout_id      TEXT    NOT NULL REFERENCES day_workout(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL CHECK (position >= 0),
    kind            TEXT    NULL,
    distance_m      REAL    NULL CHECK (distance_m >= 0),
    duration_ms     INTEGER NULL CHECK (duration_ms >= 0),
    avg_hr_bpm      INTEGER NULL CHECK (avg_hr_bpm > 0),
    pace_sec_per_km REAL    NULL CHECK (pace_sec_per_km > 0),
    PRIMARY KEY (workout_id, position)
);
