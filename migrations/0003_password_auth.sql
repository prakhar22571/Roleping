-- Password credentials for in-app login. Users with a NULL password_hash
-- exist but cannot log in until an owner sets a password for them.
ALTER TABLE users ADD COLUMN password_hash TEXT;
ALTER TABLE users ADD COLUMN password_salt TEXT;
ALTER TABLE users ADD COLUMN password_iterations INTEGER;

-- Bumping this invalidates every existing session cookie for the user
-- (used by password changes so old sessions can't outlive a reset).
ALTER TABLE users ADD COLUMN session_epoch INTEGER NOT NULL DEFAULT 1;
