CREATE TABLE users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  email      TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE company_subscriptions (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  company_id INTEGER NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (user_id, company_id)
);
CREATE INDEX idx_subscriptions_company ON company_subscriptions(company_id);

DROP TABLE application_status;
CREATE TABLE application_status (
  job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'New' CHECK (status IN ('New','Applied','Interviewing','Rejected','Offer')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (job_id, user_id)
);

DROP TABLE notifications;
CREATE TABLE notifications (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id       INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  sent_at      TEXT NOT NULL DEFAULT (datetime('now')),
  email_status TEXT NOT NULL DEFAULT 'none' CHECK (email_status IN ('sent','failed','none')),
  resend_id    TEXT,
  error_detail TEXT,
  UNIQUE (job_id, user_id)
);
CREATE INDEX idx_notifications_user ON notifications(user_id);
