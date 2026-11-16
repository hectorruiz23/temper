CREATE TABLE equipment (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE inventory_item (
    equipment_id TEXT PRIMARY KEY REFERENCES equipment(id),
    quantity     INTEGER CHECK (quantity > 0),
    max_kg       REAL    CHECK (max_kg > 0)
);

CREATE TABLE exercise (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    movement_pattern TEXT NOT NULL,
    unilateral       INTEGER CHECK (unilateral IN (0, 1)),
    created_at       INTEGER
);

CREATE TABLE exercise_equipment (
    exercise_id  TEXT REFERENCES exercise(id),
    equipment_id TEXT REFERENCES equipment(id),
    quantity     INTEGER CHECK (quantity > 0),
    PRIMARY KEY (exercise_id, equipment_id)
);

CREATE TABLE training_goal (
    id                TEXT PRIMARY KEY,
    target_date       TEXT,
    distance_m        INTEGER CHECK (distance_m > 0),
    success_criterion TEXT
);

CREATE TABLE plan_version (
    id               INTEGER PRIMARY KEY,
    created_at       INTEGER,
    reason           TEXT NOT NULL,
    effective_from   TEXT,
    timezone         TEXT,
    weekly_frequency INTEGER CHECK (weekly_frequency > 0),
    goal_id          TEXT REFERENCES training_goal(id)
);

CREATE TABLE session_template (
    id              TEXT PRIMARY KEY,
    plan_version_id INTEGER REFERENCES plan_version(id),
    position        INTEGER CHECK (position >= 0),
    scheduled_date  TEXT NULL,
    weekday         INTEGER NULL CHECK (weekday BETWEEN 1 AND 7),
    discipline      TEXT CHECK (discipline IN ('strength', 'run', 'intervals')),
    target          TEXT,
    UNIQUE (plan_version_id, position),
    CHECK ((scheduled_date IS NULL) <> (weekday IS NULL))
);

CREATE TABLE session_exercise (
    template_id  TEXT REFERENCES session_template(id),
    position     INTEGER,
    exercise_id  TEXT REFERENCES exercise(id),
    target_sets  INTEGER NULL CHECK (target_sets > 0),
    reps_min     INTEGER NULL,
    reps_max     INTEGER NULL,
    target_kg    REAL    NULL CHECK (target_kg >= 0),
    tempo        TEXT    NULL,
    PRIMARY KEY (template_id, position)
);

CREATE TABLE interval_step (
    template_id TEXT REFERENCES session_template(id),
    position    INTEGER,
    label       TEXT NOT NULL,
    duration_ms INTEGER NULL CHECK (duration_ms > 0),
    distance_m  REAL    NULL CHECK (distance_m > 0),
    PRIMARY KEY (template_id, position),
    CHECK ((duration_ms IS NULL) <> (distance_m IS NULL))
);

CREATE INDEX plan_version_effective_from ON plan_version (effective_from, id);
CREATE INDEX session_template_scheduled_date ON session_template (plan_version_id, scheduled_date);
CREATE INDEX session_template_weekday ON session_template (plan_version_id, weekday);
CREATE INDEX session_exercise_exercise ON session_exercise (exercise_id);

INSERT INTO equipment (id, name) VALUES ('adjustable-dumbbell', 'Adjustable dumbbell');
INSERT INTO inventory_item (equipment_id, quantity, max_kg) VALUES ('adjustable-dumbbell', 1, 20);
INSERT INTO training_goal (id, target_date, distance_m, success_criterion)
VALUES ('race-5k-2026-12-20', '2026-12-20', 5000, 'finish the 5K running without stopping');
