-- Takes back exactly the two rows the up added. Their roles, enrolments and any
-- work they did go with them through the cascades on users.
DELETE FROM users WHERE username IN ('superadmin', 'lukas');
