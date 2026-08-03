-- The two accounts a fresh install needs to be usable: one admin, one student.
--
-- Deliberately additive. Wiping the users table belongs in an operator command
-- run on purpose against one database (see README §11), never in a migration —
-- a migration runs on every environment it is deployed to, and one that deletes
-- users would empty production the first time it got there.
--
-- The password hash is committed to this repository, so anyone who can read the
-- repository knows these passwords. That is acceptable for the accounts a local
-- checkout starts with and not acceptable anywhere else: change them on any
-- machine other people can reach, or create the admin with
-- `make admin email=... password=...` instead and skip these.
--
-- bcrypt cost 12, the same cost internal/auth/adapter/hash uses, so a login here
-- takes exactly as long as any other.

INSERT INTO users (username, email, password_hash, status)
VALUES
    ('superadmin', 'superadmin@devforge.local',
     '$2a$12$YtNj58cMT.SO8hCoB5xJJ.kG2F2v16zmAShGuka6bXQSg.BLTMPGG', 'active'),
    ('lukas', 'lukas@devforge.local',
     '$2a$12$kg8lDMjjKMktaOifUVCPleumMkP6suUsZK0CB3ymidZhn3lvWESsG', 'active')
ON CONFLICT (username) DO NOTHING;

-- Roles by name rather than by id: the ids come from an earlier migration's
-- insert order, and pinning them here would break the day that order changes.
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
  FROM users u
  JOIN roles r ON r.name = CASE u.username
                             WHEN 'superadmin' THEN 'admin'
                             WHEN 'lukas'      THEN 'student'
                           END
 WHERE u.username IN ('superadmin', 'lukas')
ON CONFLICT DO NOTHING;
