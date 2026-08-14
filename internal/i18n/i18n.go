// Package i18n translates the messages handlers send to a browser.
//
// The Vietnamese sentence IS the key. That is deliberate: every handler already
// writes one, so keying by it means no call site has to change and no second
// name has to be invented for a string that already exists. The cost is that
// rewording a message drops its translation — which `TestCatalogueCoversSources`
// turns into a failing build rather than a silent fallback in production.
//
// Nothing here is request-scoped state: `Translate` is pure, and the language
// comes from the header on the call that is being answered.
package i18n

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Lang is one of the two languages the product ships. Anything else resolves to
// Vietnamese, which is the language the sources are written in.
type Lang string

const (
	VI Lang = "vi"
	EN Lang = "en"
)

// Translate returns the message in lang, or the message unchanged when there is
// nothing to swap it for.
//
// Falling back to Vietnamese rather than to an empty string or to the key is the
// whole safety story: an untranslated message reads exactly as it does today,
// which is worse than English and far better than blank.
func Translate(lang Lang, msg string) string {
	if lang != EN {
		return msg
	}
	if out, ok := en[msg]; ok {
		return out
	}
	return msg
}

// From reads the language off the request being answered.
//
// `?lang=` is checked first because a WebSocket opened from JavaScript cannot
// set request headers — the browser sends its own Accept-Language there, which
// is the machine's language and not the one the user picked in the app.
//
// The header is deliberately not parsed in full: the client sends exactly `vi`
// or `en` because it has exactly two languages to pick from. A browser's own
// long q-weighted header still resolves — the first tag wins — and anything
// unrecognised is Vietnamese.
func From(c *gin.Context) Lang {
	if c == nil {
		return VI
	}
	if q := c.Query("lang"); q != "" {
		return fromHeaderValue(q)
	}
	return fromHeaderValue(c.GetHeader("Accept-Language"))
}

// Split out so the parsing can be tested without building a request.
func fromHeaderValue(raw string) Lang {
	// "en-GB,en;q=0.9,vi;q=0.8" → "en-gb". Only the first tag is read: a header
	// that merely mentions English further down is a client that prefers
	// something else.
	if i := strings.IndexAny(raw, ",;"); i >= 0 {
		raw = raw[:i]
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "en") {
		return EN
	}
	return VI
}

// Msg is Translate with the language taken from the request. Every abort helper
// funnels through here, which is why no handler had to change.
func Msg(c *gin.Context, msg string) string {
	return Translate(From(c), msg)
}

// en holds one entry per message a handler can send. Kept in one map rather than
// split per module: the test that guards it scans every package at once, and a
// message moving between modules should not need the catalogue reorganised.
//
// Not translated, on purpose:
//   - the pipeline parser's sentences, which name the job, step or key at fault
//     and are built at runtime (`sim.ErrPipelineInvalid`);
//   - gin's own binding errors, which are already English.
var en = map[string]string{
	"JSON không hợp lệ":                                       "invalid JSON",
	"bài học không tồn tại":                                   "the lesson does not exist",
	"bài kiểm tra chạy quá lâu, thử lại":                      "the check took too long, try again",
	"bài lab không tồn tại":                                   "the lab does not exist",
	"bài lab này không phải bài mô phỏng":                     "this lab is not a simulation",
	"bài ôn tập không tồn tại":                                "the review note does not exist",
	"bạn cần đăng ký khoá học này trước khi làm lab":          "you must enrol in this course before starting a lab",
	"bạn đang có một phiên lab chạy dở, hãy đóng nó trước":    "you already have a lab session open, close it first",
	"chưa bắt đầu cài đặt — bấm bật lại từ đầu":               "enrolment has not started — press enable again from the top",
	"chưa có script để chạy thử":                              "there is no script to dry-run",
	"chưa cấu hình nơi lưu ảnh":                               "no image storage is configured",
	"chưa đăng nhập":                                          "not signed in",
	"chỉ khoá được tài khoản đang hoạt động":                  "only an active account can be banned",
	"chỉ nhận ảnh png, jpg, gif hoặc webp":                    "only png, jpg, gif or webp images are accepted",
	"cấp độ không hợp lệ":                                     "invalid level",
	"cần ít nhất 1 câu lệnh được chấp nhận":                   "at least 1 accepted command is required",
	"cần ít nhất 2 lựa chọn":                                  "at least 2 options are required",
	"dữ liệu không hợp lệ":                                    "invalid data",
	"email hoặc username đã tồn tại":                          "that email or username already exists",
	"forbidden":                                               "forbidden",
	"google login chưa cấu hình":                              "Google sign-in is not configured",
	"gợi ý quá dài":                                           "the hint is too long",
	"hãy chạy pipeline một lượt trước khi nộp":                "run the pipeline once before handing in",
	"hướng dẫn quá dài":                                       "the instructions are too long",
	"id không hợp lệ":                                         "invalid id",
	"image không tồn tại":                                     "the image does not exist",
	"invalid token":                                           "invalid token",
	"khoá học không tồn tại":                                  "the course does not exist",
	"không thao tác được trên chính tài khoản của bạn":        "you cannot act on your own account",
	"không thể đánh dấu tất cả là đúng":                       "they cannot all be marked correct",
	"không tìm thấy tài khoản":                                "account not found",
	"kịch bản không phải JSON hợp lệ":                         "the scenario is not valid JSON",
	"lab chưa gán image — chọn image rồi thử lại":             "the lab has no image — pick one and try again",
	"lab không tồn tại":                                       "the lab does not exist",
	"link không hợp lệ hoặc đã hết hạn":                       "the link is invalid or has expired",
	"loại nhiệm vụ không hợp lệ":                              "invalid task kind",
	"làm đúng hết các nhiệm vụ rồi mới nộp được":              "every task must pass before handing in",
	"lỗi máy chủ":                                             "server error",
	"missing token":                                           "missing token",
	"model từ chối yêu cầu này — thử mô tả lại bằng lời khác": "the model refused this request — try describing it differently",
	"mã không đúng hoặc đã hết hạn":                           "the code is wrong or has expired",
	"mã không đúng — kiểm tra lại giờ trên điện thoại":        "wrong code — check the clock on your phone",
	"mã xác thực không đúng hoặc đã hết hạn":                  "the verification code is wrong or has expired",
	"mô tả quá dài":                                           "the description is too long",
	"mật khẩu hiện tại không đúng":                            "the current password is wrong",
	"mật khẩu không đúng":                                     "wrong password",
	"một câu lệnh quá dài":                                    "one of the commands is too long",
	"một lab chỉ có thể là lab container hoặc lab mô phỏng, không thể cả hai": "a lab is either a container lab or a sim lab, never both",
	"một lượt chạy khác vừa được ghi, bấm Run lại":                            "another run was just recorded, press Run again",
	"một lựa chọn quá dài":                                          "one of the options is too long",
	"nhiệm vụ không tồn tại":                                        "the task does not exist",
	"nhiệm vụ này chưa có điều kiện chấm":                           "this task has no pass condition yet",
	"nhập sai quá nhiều lần, vui lòng đợi rồi thử lại":              "too many wrong attempts, wait and try again",
	"nội dung JSON quá dài":                                         "the JSON content is too long",
	"nội dung quá dài":                                              "the content is too long",
	"peer không hợp lệ":                                             "invalid peer",
	"phiên không tồn tại":                                           "the session does not exist",
	"phiên lab không tồn tại":                                       "the lab session does not exist",
	"phiên lab đã kết thúc":                                         "the lab session has ended",
	"phiên lab đã kết thúc, hãy mở lại bài thực hành":               "the lab session has ended, open the lab again",
	"phiên này chưa kết thúc":                                       "this session has not ended yet",
	"phiên này đã chạy hết số lượt cho phép":                        "this session has used all its allowed runs",
	"phiên này đã kết thúc rồi":                                     "this session has already ended",
	"phiên đăng nhập không hợp lệ":                                  "invalid session",
	"phải đánh dấu ít nhất 1 đáp án đúng":                           "at least 1 correct answer must be marked",
	"quá nhiều câu lệnh":                                            "too many commands",
	"quá nhiều lần thử, vui lòng đợi rồi thử lại":                   "too many attempts, wait and try again",
	"quá nhiều lựa chọn":                                            "too many options",
	"sai quá nhiều lần, hãy gửi lại mã mới":                         "too many wrong attempts, request a new code",
	"sai thông tin đăng nhập":                                       "wrong sign-in details",
	"script chạy quá 10 giây":                                       "the script ran for more than 10 seconds",
	"script quá dài":                                                "the script is too long",
	"slug chỉ gồm chữ thường, số và dấu gạch ngang":                 "a slug may only contain lowercase letters, digits and hyphens",
	"slug không được để trống":                                      "the slug cannot be empty",
	"slug này đã được dùng cho khoá khác":                           "this slug is already used by another course",
	"slug này đã được dùng cho lab khác":                            "this slug is already used by another lab",
	"slug quá dài":                                                  "the slug is too long",
	"task id không hợp lệ":                                          "invalid task id",
	"thiếu file ảnh":                                                "the image file is missing",
	"thời lượng phải từ 1 đến 600 phút":                             "the duration must be between 1 and 600 minutes",
	"thứ tự không được âm":                                          "the order cannot be negative",
	"thử thách không tồn tại":                                       "the challenge does not exist",
	"tiêu đề không được để trống":                                   "the title cannot be empty",
	"tiêu đề quá dài":                                               "the title is too long",
	"trạng thái không hợp lệ":                                       "invalid status",
	"tài khoản chưa bật xác thực hai lớp":                           "this account does not have two-factor enabled",
	"tài khoản chưa xác thực email":                                 "this account has not verified its email",
	"tài khoản đã bật xác thực hai lớp":                             "this account already has two-factor enabled",
	"tài khoản đã bật xác thực hai lớp — tắt trước rồi bật lại":     "two-factor is already on — turn it off before turning it on again",
	"tài khoản đã bị khoá":                                          "this account is banned",
	"tính năng nhờ AI dựng kịch bản chưa được bật trên máy chủ này": "AI scenario generation is not enabled on this server",
	"user không tồn tại":                                            "the user does not exist",
	"username hoặc email đã được dùng":                              "that username or email is already taken",
	"vui lòng đợi một chút rồi thử lại":                             "wait a moment and try again",
	"điểm phải từ 0 đến 1000":                                       "points must be between 0 and 1000",
	"đăng nhập sai quá nhiều lần, vui lòng đợi rồi thử lại":         "too many failed sign-ins, wait and try again",
	"đề bài không được để trống":                                    "the question cannot be empty",
	"đề bài quá dài":                                                "the question is too long",
	"ảnh phải là đường dẫn http(s)":                                 "the image must be an http(s) URL",
	"ảnh vượt quá 2MB":                                              "the image is larger than 2MB",
	"shell đã thoát":                                                "the shell exited",
	"phiên lab đã hết giờ":                                          "the lab session ran out of time",
	"không mở được terminal":                                        "could not open the terminal",
	"phiên hết hạn":                                                 "the session expired",
	"kịch bản sự cố không tồn tại":                                  "the incident scenario does not exist",
	"kịch bản này đã có người chơi — hãy tắt nó thay vì xoá":        "someone has already played this scenario — turn it off instead of deleting it",
	"kịch bản đang bật thì phải có script phá — hoặc tắt nó đi":     "an active scenario needs a break script — or turn it off",
	"lab mô phỏng không có container để dựng dịch vụ":               "a sim lab has no container to stand a service up in",
	"rps phải từ 0 đến 100000":                                      "rps must be between 0 and 100000",
	"chưa có kịch bản nào đang bật — bật một cái rồi mới đăng được": "no scenario is active yet — turn one on before publishing",
	"thử thách phải có script dựng dịch vụ":                         "a challenge needs a service setup script",
	"báo cáo này không còn được chia sẻ":                            "this report is no longer shared",
	"chưa có ca trực nào được xuất bản":                             "no on-call drill has been published yet",
	"chỉ ca trực mới có báo cáo chia sẻ được":                       "only an on-call drill has a report that can be shared",
	"kịch bản sự cố này không còn dùng được":                        "this incident scenario is no longer available",
	"tin nhắn không tồn tại":                                        "the message does not exist",
	"ngày không hợp lệ":                                             "invalid date",
	"quá nhiều yêu cầu, thử lại sau ít giây":                        "too many requests, try again in a few seconds",
	"máy chủ đang kín chỗ, thử lại sau vài phút":                    "the server is at capacity, try again in a few minutes",
}
