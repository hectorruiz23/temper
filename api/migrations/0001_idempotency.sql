CREATE TABLE idempotency_request (
    operation    TEXT    NOT NULL,
    key          TEXT    NOT NULL,
    request_hash TEXT    NOT NULL,
    result_json  TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    PRIMARY KEY (operation, key)
);
