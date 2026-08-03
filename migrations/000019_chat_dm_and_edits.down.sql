DROP INDEX IF EXISTS chat_messages_dm_participant_idx;
DROP INDEX IF EXISTS chat_messages_thread_idx;
DROP INDEX IF EXISTS chat_messages_room_idx;

ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_peer_not_self;
ALTER TABLE chat_messages
    DROP COLUMN deleted_at,
    DROP COLUMN edited_at,
    DROP COLUMN peer_id;

CREATE INDEX chat_messages_recent_idx ON chat_messages (id DESC);
