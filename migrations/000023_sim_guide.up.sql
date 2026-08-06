-- Phần "Cú pháp và luật" của một mô phỏng.
--
-- Trước đó nó là chữ nhét cứng trong frontend, viết riêng cho CI/CD: ba khoá
-- YAML, bốn luật về runner và cache. Mô phỏng thứ hai — DDoS, mạng, bất cứ thứ
-- gì — sẽ hiện ra đúng đoạn chữ đó và nó sai toàn bộ.
--
-- Cột riêng chứ không nhét vào jsonb `scenario`: đây là văn bản cho người đọc,
-- không phải thứ engine đọc. Để chung nghĩa là mỗi lần sửa một câu chữ lại phải
-- đi qua ô JSON của catalog.
ALTER TABLE sim_scenarios ADD COLUMN guide_md TEXT NOT NULL DEFAULT '';
