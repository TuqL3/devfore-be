-- Who did what to whom. Written for the actions an admin takes on somebody
-- else's account or container: those are the ones where "it just happened" is
-- not an answer anyone accepts afterwards.
--
-- Names are snapshotted alongside the ids on purpose. An admin account deleted
-- next year must not turn a year of entries into "someone banned this user" —
-- the id goes null with the row it points at, the name stays.
CREATE TABLE audit_logs (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    actor_name  TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    -- 'user' or 'lab_session'. Kept as text rather than an enum: an audit table
    -- that refuses to record an action it has not seen before is an audit table
    -- that loses the interesting ones.
    target_type TEXT        NOT NULL,
    target_id   TEXT        NOT NULL,
    target_name TEXT        NOT NULL DEFAULT '',
    -- Free text: the ban reason, or whatever else the action carried.
    detail      TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The list reads newest first, which is the only order anyone asks for.
CREATE INDEX audit_logs_recent_idx ON audit_logs (created_at DESC);
-- "What happened to this account" is the second question, always right after
-- somebody notices they cannot log in.
CREATE INDEX audit_logs_target_idx ON audit_logs (target_type, target_id, created_at DESC);
