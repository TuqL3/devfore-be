-- Direct messages, and the ability to take a message back.
--
-- No conversations table. A direct message is a row with a peer_id; the shared
-- room is a row without one. A pair is identified by its two ids sorted, which
-- the index below makes cheap — a conversations table would be a second place
-- for the same fact to live and a join on every read.
ALTER TABLE chat_messages
    -- CASCADE rather than SET NULL, unlike user_id above it: room history has
    -- to survive its author being deleted, but a private message to an account
    -- that no longer exists has nobody left to read it.
    ADD COLUMN peer_id BIGINT REFERENCES users (id) ON DELETE CASCADE,
    -- Null until edited. The screen says "đã sửa" off this rather than
    -- comparing timestamps, which would call every message edited the moment
    -- created_at and updated_at differ by a microsecond.
    ADD COLUMN edited_at TIMESTAMPTZ,
    -- Soft: the row stays so ids and ordering do not reflow under everyone
    -- else's scroll position, and the body is blanked so the content is
    -- actually gone rather than merely hidden by the client.
    ADD COLUMN deleted_at TIMESTAMPTZ;

-- A message cannot be addressed to its own sender. Cheap to state here, and it
-- is the one shape that would make LEAST = GREATEST and collapse a thread into
-- itself.
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_peer_not_self
    CHECK (peer_id IS NULL OR peer_id <> user_id);

-- The shared room's read. Partial, so direct messages never touch it.
DROP INDEX IF EXISTS chat_messages_recent_idx;
CREATE INDEX chat_messages_room_idx
    ON chat_messages (id DESC) WHERE peer_id IS NULL;

-- One thread, whichever direction each message went. The expression is what
-- lets the pair be a key without storing it twice.
CREATE INDEX chat_messages_thread_idx
    ON chat_messages (LEAST(user_id, peer_id), GREATEST(user_id, peer_id), id DESC)
    WHERE peer_id IS NOT NULL;

-- "Which threads do I have" — answered from either side of the pair.
CREATE INDEX chat_messages_dm_participant_idx
    ON chat_messages (user_id, peer_id, id DESC) WHERE peer_id IS NOT NULL;
