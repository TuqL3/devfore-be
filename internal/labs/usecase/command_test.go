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
		{"khác tham số", history, "ls -al /", false},
		{"tiền tố không tính", history, "ls", false},
		{"history rỗng", "", "uname -a", false},
		{"đáp án rỗng", history, "", false},
		{"đáp án chỉ có khoảng trắng", history, "   \n\t", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ranCommand(c.history, c.expected); got != c.want {
				t.Errorf("ranCommand(%q, %q) = %v, want %v", c.history, c.expected, got, c.want)
			}
		})
	}
}
