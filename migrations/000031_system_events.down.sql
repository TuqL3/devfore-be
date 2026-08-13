-- Dropping the table loses the history of what went wrong. Nothing else reads
-- it, so nothing else breaks — the admin screen simply stops having a feed.
DROP TABLE IF EXISTS system_events;
