-- Nội dung mô phỏng ở sân chơi chuyển vào code (`devforge-fe/src/sims/`).
--
-- Lý do: nội dung đó **là** code. Một form quản trị chỉ đẻ ra được thứ nó biết
-- trước hình dạng, mà mô phỏng thứ hai — hàng đợi, vòng lặp agent, gì khác —
-- có hình dạng khác hẳn. Giữ hai bảng này nghĩa là mỗi mô phỏng mới phải có một
-- hàng DB và một component, hai chỗ giữ khớp nhau mà không ai bắt được khi lệch.
--
-- MẤT DỮ LIỆU: mọi kịch bản sân chơi và lời mở đầu đã sửa qua giao diện. Hai
-- kịch bản seed đã nằm trong `src/sims/cicd.ts` nên không mất gì; kịch bản ai đó
-- tự viết qua màn quản trị (màn đó nay đã xoá) thì mất.
--
-- KHÔNG đụng lab mô phỏng có chấm điểm: nó dùng `labs.sim_scenario`, là cột
-- khác, và ở đó nội dung thật sự là dữ liệu — tác giả không biết code vẫn viết
-- được qua Quản trị → Khoá học.
DROP TABLE sim_scenarios;
DROP TABLE app_settings;
