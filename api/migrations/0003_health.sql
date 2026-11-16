CREATE TABLE day (
    date             TEXT    PRIMARY KEY,
    timezone         TEXT    NOT NULL,
    revision         INTEGER NOT NULL CHECK(revision > 0),
    sleep_ms         INTEGER NULL CHECK(sleep_ms >= 0),
    steps            INTEGER NULL CHECK(steps >= 0),
    resting_hr_bpm   INTEGER NULL CHECK(resting_hr_bpm > 0),
    hrv_rmssd_ms     REAL    NULL CHECK(hrv_rmssd_ms >= 0),
    updated_at       INTEGER NOT NULL,
    last_source      TEXT    NOT NULL CHECK(last_source IN ('manual', 'health_connect'))
);

CREATE TABLE day_sleep_stage (
    day_date    TEXT    REFERENCES day(date),
    stage       TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL CHECK(duration_ms >= 0),
    PRIMARY KEY (day_date, stage)
);

CREATE TABLE day_workout (
    id             TEXT    PRIMARY KEY,
    day_date       TEXT    REFERENCES day(date),
    origin         TEXT    NOT NULL,
    external_id    TEXT    NOT NULL,
    device_kind    TEXT    NULL,
    started_at     INTEGER NOT NULL,
    ended_at       INTEGER NOT NULL,
    discipline     TEXT    NOT NULL,
    duration_ms    INTEGER NOT NULL CHECK(duration_ms >= 0),
    distance_m     REAL    NULL CHECK(distance_m >= 0),
    watch_verified INTEGER NOT NULL CHECK(watch_verified IN (0, 1)),
    present        INTEGER NOT NULL CHECK(present IN (0, 1)),
    UNIQUE (origin, external_id)
);

CREATE INDEX day_workout_day_date_started_at ON day_workout(day_date, started_at);
