package usecase

import (
	"strconv"
	"testing"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// The timeline is the whole of what the report teaches, and every way of getting
// it wrong is quiet: a dropped line hides an attempt somebody made, a stamp read
// as a command invents one they never typed, and a stamp reused for the next
// command dates it to a moment that has nothing to do with it.
func TestParseTimeline(t *testing.T) {
	cases := []struct {
		name    string
		history string
		want    []string // "epoch|command", epoch 0 for an entry with no time
	}{
		{
			name:    "stamped history, one stamp per command",
			history: "#1786400000\ndf -h\n#1786400042\nrm /tmp/big\n",
			want:    []string{"1786400000|df -h", "1786400042|rm /tmp/big"},
		},
		{
			// A session that started before the rc file set HISTTIMEFORMAT. The
			// commands still have to show up; only the times are missing.
			name:    "unstamped history still lists every command",
			history: "ls -l\ncat /etc/hosts\n",
			want:    []string{"0|ls -l", "0|cat /etc/hosts"},
		},
		{
			// A comment the student typed is a command they ran, not a stamp, so
			// it stays in the record — and it keeps the stamp bash wrote for it,
			// because bash stamps every history line including that one. Reading
			// it as a stamp instead would delete a line somebody typed.
			name:    "a typed comment is a command, and keeps its own stamp",
			history: "#1786400000\n# note to self\nls\n",
			want:    []string{"1786400000|# note to self", "0|ls"},
		},
		{
			// The stamp belongs to the command right after it. Carrying it
			// forward would date an untimed line to a moment it did not happen.
			name:    "a stamp is spent on one command only",
			history: "#1786400000\nfirst\nsecond\n",
			want:    []string{"1786400000|first", "0|second"},
		},
		{
			name:    "blank lines and trailing newlines are not commands",
			history: "\n#1786400000\n\ndf -h\n\n",
			want:    []string{"1786400000|df -h"},
		},
		{
			name:    "empty history is an empty timeline, not a nil one",
			history: "",
			want:    []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseTimeline(c.history)
			if len(got) != len(c.want) {
				t.Fatalf("số dòng: muốn %d, nhận %d (%+v)", len(c.want), len(got), got)
			}
			for i, want := range c.want {
				var sec int64
				if !got[i].At.IsZero() {
					sec = got[i].At.Unix()
				}
				have := strconv.FormatInt(sec, 10) + "|" + got[i].Command
				if have != want {
					t.Fatalf("dòng %d: muốn %q, nhận %q", i, want, have)
				}
			}
		})
	}
}

// The deadline is the challenge. Getting it wrong in one direction hands the
// student an hour for a fifteen-minute drill; in the other it lets a lab outlive
// the container TTL the host agreed to, which is the one bound that is not about
// pedagogy at all.
func TestDrillDeadline(t *testing.T) {
	const ttl = 60 * time.Minute

	cases := []struct {
		name string
		spec domain.Spec
		want time.Duration
	}{
		{
			name: "lab thường: TTL của nền tảng, không phải duration_minutes",
			spec: domain.Spec{IsIncident: false, DurationMinutes: 45},
			want: ttl,
		},
		{
			name: "drill: hạn riêng của thử thách",
			spec: domain.Spec{IsIncident: true, DurationMinutes: 15},
			want: 15 * time.Minute,
		},
		{
			// Tác giả gõ 999 phút thì container vẫn không được sống lâu hơn thứ
			// máy chủ đồng ý cho nó sống.
			name: "drill dài hơn TTL vẫn bị TTL chặn",
			spec: domain.Spec{IsIncident: true, DurationMinutes: 999},
			want: ttl,
		},
		{
			// Cột mặc định 60, nhưng 0 là dữ liệu hỏng — rơi về TTL chứ không
			// tạo một phiên hết hạn ngay lúc vừa bắt đầu.
			name: "drill khai 0 phút: rơi về TTL",
			spec: domain.Spec{IsIncident: true, DurationMinutes: 0},
			want: ttl,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := drillDeadline(&c.spec, ttl); got != c.want {
				t.Fatalf("muốn %v, nhận %v", c.want, got)
			}
		})
	}
}

// Recovery is the moment the whole report is built around: MTTR, the cost, and
// whether the drill counts as survived. Claiming one that did not happen is the
// worst thing this screen can do, so the cases that must answer "never" are the
// ones worth pinning down.
func TestRecoveredAt(t *testing.T) {
	yes, no := true, false
	at := func(sec int64) *time.Time { t := time.Unix(sec, 0).UTC(); return &t }

	cases := []struct {
		name    string
		answers []domain.ReportAnswer
		want    *time.Time
	}{
		{
			// More than one task: the outage is not over while any of them is
			// still failing, so the last green one is the recovery.
			name: "mọi task đậu — lấy mốc muộn nhất",
			answers: []domain.ReportAnswer{
				{Passed: &yes, AnsweredAt: at(1786400000)},
				{Passed: &yes, AnsweredAt: at(1786400400)},
			},
			want: at(1786400400),
		},
		{
			name: "còn task trượt — chưa cứu được",
			answers: []domain.ReportAnswer{
				{Passed: &yes, AnsweredAt: at(1786400000)},
				{Passed: &no, AnsweredAt: at(1786400400)},
			},
			want: nil,
		},
		{
			// Ran out of time with a task never checked. A partial pass must not
			// be dated as a recovery.
			name: "có task chưa bấm kiểm tra lần nào",
			answers: []domain.ReportAnswer{
				{Passed: &yes, AnsweredAt: at(1786400000)},
				{Passed: nil, AnsweredAt: nil},
			},
			want: nil,
		},
		{
			// A lab with no tasks would otherwise vacuously "recover" at the zero
			// time, which reads as an outage that ended in 1970.
			name:    "lab không có task nào",
			answers: []domain.ReportAnswer{},
			want:    nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RecoveredAt(&domain.Report{Answers: c.answers})
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("muốn chưa cứu được, nhận %v", got)
			case c.want != nil && got == nil:
				t.Fatalf("muốn %v, nhận chưa cứu được", c.want)
			case c.want != nil && !got.Equal(*c.want):
				t.Fatalf("muốn %v, nhận %v", c.want, got)
			}
		})
	}
}

// The cost line is the one number on the report a student could quote at
// somebody, so it fails closed: nothing is claimed from a clock that ran
// backwards or a scenario whose author left the rate at zero.
func TestRequestsFailed(t *testing.T) {
	if got := RequestsFailed(400*time.Second, 20); got != 8000 {
		t.Fatalf("6' 40\" ở 20 rps: muốn 8000, nhận %d", got)
	}
	// Sub-second downtime truncates rather than rounding up: claiming failures
	// for an outage nobody could see would be the report lying first.
	if got := RequestsFailed(900*time.Millisecond, 20); got != 0 {
		t.Fatalf("dưới một giây: muốn 0, nhận %d", got)
	}
	if got := RequestsFailed(-5*time.Second, 20); got != 0 {
		t.Fatalf("thời lượng âm: muốn 0, nhận %d", got)
	}
	if got := RequestsFailed(60*time.Second, 0); got != 0 {
		t.Fatalf("rps = 0: muốn 0, nhận %d", got)
	}
}
