-- Second factor, TOTP (RFC 6238).
--
-- The secret is stored as the app reads it. Encrypting it with a key that ships
-- in the same deployment as the database buys little real separation — anyone
-- who can read this table can read the key — and it adds a failure mode that is
-- worse than the threat: rotating that key would leave every enrolled user
-- unable to produce a valid code, with only their single-use recovery codes
-- between them and a locked account. If this ever needs to change, the answer is
-- a key held somewhere the database is not, not a constant in the config.
--
-- What does protect it: the secret is never returned once enrolment is
-- confirmed, and a stolen secret alone is not a login — the password is still
-- the first factor.
CREATE TABLE user_totp (
    user_id      BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret       TEXT        NOT NULL,
    -- Null while enrolment is half done: the secret exists and has been shown,
    -- but no code has proved the authenticator holds it. Only a confirmed row
    -- makes login ask for a code, so an abandoned enrolment cannot lock anyone
    -- out of their own account.
    confirmed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One-time codes for the day the phone is gone. Hashed with the same bcrypt the
-- passwords use: they are as good as a password while unused, so they are stored
-- the same way.
CREATE TABLE user_recovery_codes (
    id        BIGSERIAL   PRIMARY KEY,
    user_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash TEXT        NOT NULL,
    -- Set the moment it is spent. Kept rather than deleted so "somebody used a
    -- recovery code" is answerable afterwards.
    used_at   TIMESTAMPTZ
);

CREATE INDEX user_recovery_codes_user_idx ON user_recovery_codes (user_id) WHERE used_at IS NULL;
