CREATE TABLE training_session (
    id                       TEXT    PRIMARY KEY,
    template_id              TEXT    NULL REFERENCES session_template(id),
    date                     TEXT    NOT NULL,
    discipline               TEXT    NOT NULL CHECK(discipline IN ('strength', 'run', 'intervals')),
    started_at               INTEGER NULL,
    reported_completed_at    INTEGER NULL,
    watch_workout_id         TEXT    NULL UNIQUE REFERENCES day_workout(id),
    created_at               INTEGER NOT NULL,
    revision                 INTEGER NOT NULL CHECK(revision > 0)
);

CREATE TABLE logged_set (
    id               TEXT    PRIMARY KEY,
    session_id       TEXT    NOT NULL REFERENCES training_session(id),
    exercise_id       TEXT    NOT NULL REFERENCES exercise(id),
    set_index        INTEGER NOT NULL CHECK(set_index > 0),
    kg               REAL    NOT NULL CHECK(kg >= 0),
    reps             INTEGER NOT NULL CHECK(reps > 0),
    note             TEXT    NULL,
    evidence_source  TEXT    NOT NULL CHECK(evidence_source IN ('watch', 'reported')),
    idempotency_key  TEXT    NULL,
    created_at       INTEGER NOT NULL,
    revision         INTEGER NOT NULL CHECK(revision > 0),
    UNIQUE (session_id, exercise_id, set_index)
);

CREATE INDEX training_session_date ON training_session (date);
CREATE INDEX training_session_template ON training_session (template_id);
CREATE INDEX logged_set_exercise_created ON logged_set (exercise_id, created_at);
