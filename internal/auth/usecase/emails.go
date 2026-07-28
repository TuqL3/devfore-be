package usecase

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

// Plain inline-styled HTML on purpose: mail clients strip <style> blocks and
// ignore most of a stylesheet, so anything fancier would render worse, not
// better.
const shell = `<div style="font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;max-width:480px;margin:0 auto;padding:24px;color:#27272a">
<h1 style="font-size:20px;margin:0 0 16px">DevForge</h1>%s
<p style="margin:24px 0 0;font-size:12px;color:#71717a">Không phải bạn? Bỏ qua email này.</p>
</div>`

func codeEmail(code string, ttl time.Duration) string {
	return fmt.Sprintf(shell, fmt.Sprintf(
		`<p style="margin:0 0 8px">Mã xác thực tài khoản của bạn:</p>
<p style="font-family:ui-monospace,monospace;font-size:32px;font-weight:700;letter-spacing:8px;margin:0 0 8px">%s</p>
<p style="margin:0;font-size:14px;color:#52525b">Mã hết hạn sau %s.</p>`,
		template.HTMLEscapeString(code), minutes(ttl)))
}

func resetEmail(link string, ttl time.Duration) string {
	return fmt.Sprintf(shell, fmt.Sprintf(
		`<p style="margin:0 0 16px">Bấm nút bên dưới để đặt lại mật khẩu:</p>
<p style="margin:0 0 16px"><a href="%s" style="display:inline-block;background:#b45309;color:#fff;text-decoration:none;padding:12px 20px;border-radius:6px;font-weight:600">Đặt lại mật khẩu</a></p>
<p style="margin:0;font-size:14px;color:#52525b">Link chỉ dùng được một lần và hết hạn sau %s.</p>`,
		template.HTMLEscapeString(link), minutes(ttl)))
}

func googleOnlyEmail() string {
	return fmt.Sprintf(shell,
		`<p style="margin:0">Tài khoản này đăng nhập bằng Google nên không có mật khẩu để đặt lại. Dùng nút "Đăng nhập với Google" ở trang đăng nhập.</p>`)
}

// "10 phút" reads better than "10m0s" in a mail.
func minutes(d time.Duration) string {
	if d >= time.Hour {
		return strings.TrimSuffix(fmt.Sprintf("%.0f", d.Hours()), ".0") + " giờ"
	}
	return fmt.Sprintf("%.0f", d.Minutes()) + " phút"
}
