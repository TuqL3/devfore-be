-- One global room. No room id column: the product has one room, and a column
-- that is the same value on every row is a column that has to be explained.
-- Splitting into rooms later is a migration, not a thing to carry now.
CREATE TABLE chat_messages (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    -- Snapshotted for the same reason the audit log does it: a message whose
    -- author was deleted still has to say who wrote it, and history that
    -- silently reattributes itself is worse than history that is gone.
    username   TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The only read there is: the tail, newest first, then reversed for display.
CREATE INDEX chat_messages_recent_idx ON chat_messages (id DESC);
