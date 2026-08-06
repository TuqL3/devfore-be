-- Vài mẩu chữ thuộc về cả trang chứ không thuộc hàng nào: lời giới thiệu ở đầu
-- danh sách mô phỏng là cái đầu tiên. Trước đó nó nằm cứng trong frontend, nên
-- sửa một câu là phải deploy.
--
-- Bảng khoá–giá trị chứ không phải một bảng riêng cho một chuỗi: cái sau đọc
-- gọn hơn hôm nay và thành mười bảng một cột vào năm sau. Khoá là chuỗi do code
-- đặt, không phải do người dùng nhập, nên không có gì để kiểm ở đây.
CREATE TABLE app_settings (
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
