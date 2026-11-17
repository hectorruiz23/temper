-- A session id was the primary key of session_template, so an id could exist
-- only once in the whole database. Changing a session in a new plan version
-- therefore forced the caller to invent a fresh id for an otherwise identical
-- session, and the previous version lost the link. Ids are now unique per plan
-- version, and every child row carries the version it belongs to.
--
-- The rebuild happens inside the migration transaction, so the checks are
-- deferred until commit: the tables are dropped and recreated under their
-- final names and only the committed state has to be consistent.
PRAGMA defer_foreign_keys = ON;

CREATE TABLE session_template_keys (
    id              TEXT NOT NULL,
    plan_version_id INTEGER NOT NULL REFERENCES plan_version(id),
    position        INTEGER CHECK (position >= 0),
    scheduled_date  TEXT NULL,
    weekday         INTEGER NULL CHECK (weekday BETWEEN 1 AND 7),
    discipline      TEXT CHECK (discipline IN ('strength', 'run', 'intervals')),
    target          TEXT,
    PRIMARY KEY (plan_version_id, id),
    UNIQUE (plan_version_id, position),
    CHECK ((scheduled_date IS NULL) <> (weekday IS NULL))
);

INSERT INTO session_template_keys (id, plan_version_id, position, scheduled_date, weekday, discipline, target)
SELECT id, plan_version_id, position, scheduled_date, weekday, discipline, target FROM session_template;

CREATE TABLE session_exercise_keys (
    plan_version_id INTEGER NOT NULL,
    template_id     TEXT NOT NULL,
    position        INTEGER,
    exercise_id     TEXT REFERENCES exercise(id),
    target_sets     INTEGER NULL CHECK (target_sets > 0),
    reps_min        INTEGER NULL,
    reps_max        INTEGER NULL,
    target_kg       REAL NULL CHECK (target_kg >= 0),
    tempo           TEXT NULL,
    PRIMARY KEY (plan_version_id, template_id, position),
    FOREIGN KEY (plan_version_id, template_id) REFERENCES session_template_keys(plan_version_id, id)
);

INSERT INTO session_exercise_keys (plan_version_id, template_id, position, exercise_id, target_sets, reps_min, reps_max, target_kg, tempo)
SELECT st.plan_version_id, se.template_id, se.position, se.exercise_id, se.target_sets, se.reps_min, se.reps_max, se.target_kg, se.tempo
FROM session_exercise se
JOIN session_template st ON st.id = se.template_id;

CREATE TABLE interval_step_keys (
    plan_version_id INTEGER NOT NULL,
    template_id     TEXT NOT NULL,
    position        INTEGER,
    label           TEXT NOT NULL,
    duration_ms     INTEGER NULL CHECK (duration_ms > 0),
    distance_m      REAL NULL CHECK (distance_m > 0),
    PRIMARY KEY (plan_version_id, template_id, position),
    FOREIGN KEY (plan_version_id, template_id) REFERENCES session_template_keys(plan_version_id, id),
    CHECK ((duration_ms IS NULL) <> (distance_m IS NULL))
);

INSERT INTO interval_step_keys (plan_version_id, template_id, position, label, duration_ms, distance_m)
SELECT st.plan_version_id, istep.template_id, istep.position, istep.label, istep.duration_ms, istep.distance_m
FROM interval_step istep
JOIN session_template st ON st.id = istep.template_id;

CREATE TABLE training_session_keys (
    id                    TEXT PRIMARY KEY,
    plan_version_id       INTEGER NULL,
    template_id           TEXT NULL,
    date                  TEXT NOT NULL,
    discipline            TEXT NOT NULL CHECK (discipline IN ('strength', 'run', 'intervals')),
    started_at            INTEGER NULL,
    reported_completed_at INTEGER NULL,
    watch_workout_id      TEXT NULL UNIQUE REFERENCES day_workout(id),
    created_at            INTEGER NOT NULL,
    revision              INTEGER NOT NULL CHECK (revision > 0),
    FOREIGN KEY (plan_version_id, template_id) REFERENCES session_template_keys(plan_version_id, id)
);

INSERT INTO training_session_keys (id, plan_version_id, template_id, date, discipline, started_at, reported_completed_at, watch_workout_id, created_at, revision)
SELECT ts.id, st.plan_version_id, ts.template_id, ts.date, ts.discipline, ts.started_at, ts.reported_completed_at, ts.watch_workout_id, ts.created_at, ts.revision
FROM training_session ts
LEFT JOIN session_template st ON st.id = ts.template_id;

DROP TABLE session_exercise;
DROP TABLE interval_step;
DROP TABLE training_session;
DROP TABLE session_template;

ALTER TABLE session_template_keys RENAME TO session_template;
ALTER TABLE session_exercise_keys RENAME TO session_exercise;
ALTER TABLE interval_step_keys RENAME TO interval_step;
ALTER TABLE training_session_keys RENAME TO training_session;

CREATE INDEX session_template_scheduled_date ON session_template (plan_version_id, scheduled_date);
CREATE INDEX session_template_weekday ON session_template (plan_version_id, weekday);
CREATE INDEX session_exercise_exercise ON session_exercise (exercise_id);
CREATE INDEX training_session_date ON training_session (date);
CREATE INDEX training_session_template ON training_session (template_id);
