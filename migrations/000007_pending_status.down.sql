-- Nothing can hold 'pending' once the constraint is back, and an unverified
-- signup is closer to an active account than to a banned one.
UPDATE users SET status = 'active' WHERE status = 'pending';
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'banned'));
