package rest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devforge/be/internal/courses/domain"
)

// lab_tasks.check_script decides whether a submission passes. Serving it to the
// client hands out the answers, so the guard is that no field of the response
// can carry it — not that the query happens to leave it out today.
func TestLabDetailNeverSerialisesCheckScript(t *testing.T) {
	lab := &domain.Lab{
		ID: 7, Slug: "dieu-huong-filesystem", Title: "Điều Hướng Filesystem",
		DescriptionMD: "cd vào /var/log", DurationMinutes: 60, TaskCount: 2, Points: 25,
		Tasks: []domain.Task{
			{ID: 1, Title: "cd /var/log", Points: 10, OrderIdx: 0},
			{ID: 2, Title: "ls -la", Points: 15, OrderIdx: 1},
		},
	}

	b, err := json.Marshal(newLabDetail(lab))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(b)

	for _, leak := range []string{"check_script", "checkScript", "CheckScript"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries %q:\n%s", leak, body)
		}
	}

	var got labDetail
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(got.Tasks))
	}
	if got.Tasks[0].Title != "cd /var/log" || got.Tasks[1].Points != 15 {
		t.Errorf("tasks did not round-trip: %+v", got.Tasks)
	}
	if got.Slug != lab.Slug || got.Points != 25 {
		t.Errorf("lab fields did not round-trip: %+v", got.labResponse)
	}
}

// An empty task list has to marshal as [] and not null: the frontend maps over
// it, and null is a runtime error there rather than an empty render.
func TestLabDetailWithNoTasksMarshalsEmptyArray(t *testing.T) {
	b, err := json.Marshal(newLabDetail(&domain.Lab{Slug: "quyen-so-huu"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tasks":[]`) {
		t.Errorf(`want "tasks":[], got: %s`, b)
	}
}
