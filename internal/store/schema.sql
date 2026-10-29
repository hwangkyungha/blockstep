CREATE TABLE IF NOT EXISTS scenario (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS branch (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    from_step_id INTEGER NOT NULL REFERENCES step(id),
    label        TEXT    NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS step (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    scenario_id      INTEGER NOT NULL REFERENCES scenario(id),
    parent_branch_id INTEGER REFERENCES branch(id),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    name             TEXT    NOT NULL,
    refs             TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'unchecked'
                     CHECK (status IN ('supported', 'blocked', 'needs_test', 'not_implemented', 'unchecked')),
    checked_package  TEXT    NOT NULL DEFAULT '',
    checked_by       TEXT    NOT NULL DEFAULT '',
    checked_at       TEXT
);

CREATE TABLE IF NOT EXISTS issue (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    title           TEXT    NOT NULL,
    description     TEXT    NOT NULL DEFAULT '',
    reporter        TEXT    NOT NULL,
    assignee        TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open', 'assigned', 'in_progress', 'waiting_package', 'fixed', 'verified', 'on_hold', 'rejected')),
    priority        TEXT    NOT NULL CHECK (priority IN ('P1', 'P2', 'P3')),
    found_package   TEXT    NOT NULL,
    applied_package TEXT    NOT NULL DEFAULT '',
    due_date        TEXT,
    resolution_note TEXT    NOT NULL DEFAULT '',
    jira_key        TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS issue_step (
    issue_id INTEGER NOT NULL REFERENCES issue(id),
    step_id  INTEGER NOT NULL REFERENCES step(id),
    PRIMARY KEY (issue_id, step_id)
);

CREATE TABLE IF NOT EXISTS issue_note (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   INTEGER NOT NULL REFERENCES issue(id),
    author     TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS change_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    entity    TEXT    NOT NULL,
    entity_id INTEGER NOT NULL,
    field     TEXT    NOT NULL,
    old_value TEXT    NOT NULL DEFAULT '',
    new_value TEXT    NOT NULL DEFAULT '',
    actor     TEXT    NOT NULL DEFAULT '',
    note      TEXT    NOT NULL DEFAULT '',
    at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS person (
    name TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO setting (key, value) VALUES ('stale_days', '14');
