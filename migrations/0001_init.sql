CREATE TABLE companies (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  portal_url    TEXT NOT NULL,
  adapter_type  TEXT NOT NULL CHECK (adapter_type IN ('amazon','greenhouse','lever')),
  adapter_config TEXT,
  is_active     INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE jobs (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  external_id         TEXT NOT NULL,
  source              TEXT NOT NULL,
  company_id          INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  title               TEXT NOT NULL,
  location            TEXT,
  posted_date         TEXT,
  qualifications_text TEXT,
  apply_url           TEXT NOT NULL,
  raw_json            TEXT,
  first_seen_at       TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (source, external_id)
);
CREATE INDEX idx_jobs_company ON jobs(company_id);
CREATE INDEX idx_jobs_first_seen ON jobs(first_seen_at);

CREATE TABLE llm_verdicts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id        INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  model_id      TEXT NOT NULL,
  match         INTEGER NOT NULL,
  confidence    REAL,
  reasoning     TEXT NOT NULL,
  raw_response  TEXT,
  classified_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (job_id, model_id)
);
CREATE INDEX idx_verdicts_job ON llm_verdicts(job_id);

CREATE TABLE notifications (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id       INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  sent_at      TEXT NOT NULL DEFAULT (datetime('now')),
  email_status TEXT NOT NULL CHECK (email_status IN ('sent','failed')),
  resend_id    TEXT,
  error_detail TEXT,
  UNIQUE (job_id)
);

CREATE TABLE application_status (
  job_id     INTEGER PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'New' CHECK (status IN ('New','Applied','Interviewing','Rejected','Offer')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
