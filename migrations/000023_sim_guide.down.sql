-- Mất phần hướng dẫn của mọi kịch bản. Seed đặt lại được cho những cái nó tạo,
-- còn kịch bản do tác giả viết thì mất hẳn.
ALTER TABLE sim_scenarios DROP COLUMN guide_md;
