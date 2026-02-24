-- Research sessions (one per user query)
CREATE TABLE IF NOT EXISTS research_sessions (
    id              TEXT PRIMARY KEY,
    topic           TEXT NOT NULL,
    status          TEXT NOT NULL, -- queued | searching | structuring | writing | complete | failed
    error           TEXT,
    report_md_key   TEXT,          -- object storage key
    report_json_key TEXT,          -- object storage key
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Key findings from StructuredResearch
CREATE TABLE IF NOT EXISTS key_findings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES research_sessions(id),
    finding     TEXT NOT NULL,
    confidence  REAL NOT NULL
);

-- Open questions from StructuredResearch
CREATE TABLE IF NOT EXISTS open_questions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES research_sessions(id),
    question    TEXT NOT NULL
);

-- Sources (replaces sources.csv)
CREATE TABLE IF NOT EXISTS sources (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES research_sessions(id),
    query       TEXT NOT NULL,
    url         TEXT NOT NULL,
    snippet     TEXT
);
