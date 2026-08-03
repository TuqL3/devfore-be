package usecase

import "testing"

// The whole of a command task's grading is this comparison, and getting it wrong
// either fails a student who typed the command or passes one who did not.
func TestRanCommand(t *testing.T) {
	const history = "cd /var/log\nuname   -a\nls -la /\n"

	cases := []struct {
		name     string
		history  string
		expected string
		want     bool
	}{
		{"khớp chính xác", history, "ls -la /", true},
		{"thừa khoảng trắng trong history", history, "uname -a", true},
		{"thừa khoảng trắng trong đáp án", history, "uname    -a", true},
		{"một trong nhiều đáp án", history, "uname --all\nuname -a", true},
		{"chưa gõ", history, "which bash", false},
		{"tiền tố không tính", history, "ls", false},
		{"history rỗng", "", "uname -a", false},
		{"đáp án rỗng", history, "", false},
		{"đáp án chỉ có khoảng trắng", history, "   \n\t", false},

		// Cụm cờ ngắn: với người gõ đây là cùng một lệnh, nên cùng một đáp án.
		{"đảo thứ tự trong cụm cờ", history, "ls -al /", true},
		{"tách cụm cờ ra rời", history, "ls -a -l /", true},
		{"tách và đảo", history, "ls -l -a /", true},
		{"đáp án gộp, history rời", "ls -a -l /\n", "ls -la /", true},

		// Cờ dài một gạch bị tách thành chữ cái y hệt cụm cờ ngắn. Không sao:
		// hai vế cùng qua một hàm nên vẫn khớp, và cờ không nhảy qua đối số của
		// nó sang chỗ khác.
		{"cờ dài một gạch vẫn khớp", "find / -name passwd -type f\n", "find / -name passwd -type f", true},
		{"cờ không nhảy qua đối số", "find / -name passwd -type f\n", "find / -type passwd -name f", false},

		// Cờ dính số không tách, nếu không `-n5` thành `-5 -n` và lệch hẳn.
		{"cờ dính số giữ nguyên", "head -n5 /etc/passwd\n", "head -n5 /etc/passwd", true},
		{"cờ dính số vẫn khác cách viết rời", "head -n5 /etc/passwd\n", "head -n 5 /etc/passwd", false},

		// Cờ hai gạch không phải cụm cờ ngắn.
		{"cờ hai gạch không bị tách", "ls --all /\n", "ls --all /", true},
		{"cờ hai gạch khác cờ ngắn", "ls --all /\n", "ls -a /", false},

		// Nới rộng có giá của nó: lệnh này chạy sẽ lỗi nhưng vẫn được tính.
		{"đổi chỗ trong cụm cờ được chấp nhận", "tar -cfz a.tar b\n", "tar -czf a.tar b", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ranCommand(c.history, c.expected); got != c.want {
				t.Errorf("ranCommand(%q, %q) = %v, want %v", c.history, c.expected, got, c.want)
			}
		})
	}
}
