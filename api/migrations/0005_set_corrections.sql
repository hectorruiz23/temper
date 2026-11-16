CREATE TABLE set_correction (
    id                 TEXT    PRIMARY KEY,
    set_id             TEXT    NOT NULL REFERENCES logged_set(id),
    created_at         INTEGER NOT NULL,
    reason             TEXT    NOT NULL,
    before_json        TEXT    NOT NULL,
    after_json         TEXT    NOT NULL,
    resulting_revision INTEGER NOT NULL,
    idempotency_key    TEXT    NULL,
    UNIQUE (set_id, resulting_revision)
);

CREATE INDEX set_correction_set_created ON set_correction (set_id, created_at);
