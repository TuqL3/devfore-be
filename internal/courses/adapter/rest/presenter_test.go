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

// A choice question carries its options to the client — it cannot be answered
// otherwise — but which of them is right is the same class of secret as a check
// script. domain.Task has no field for it at all; this pins that down, so
// adding one to make some future screen easier fails here first.
func TestLabDetailNeverSerialisesCorrectAnswers(t *testing.T) {
	lab := &domain.Lab{
		Slug: "ly-thuyet",
		Tasks: []domain.Task{{
			ID: 3, Title: "Lệnh nào liệt kê file?", Points: 5,
			Kind:    domain.KindChoice,
			Options: []string{"ls", "cd", "rm"},
		}},
	}

	b, err := json.Marshal(newLabDetail(lab))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(b)

	for _, leak := range []string{"correct", "Correct"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries %q:\n%s", leak, body)
		}
	}
	if !strings.Contains(body, `"options":["ls","cd","rm"]`) {
		t.Errorf("option text should reach the student, got: %s", body)
	}
	if !strings.Contains(body, `"kind":"choice"`) {
		t.Errorf("kind should reach the student, got: %s", body)
	}
}

// lab_tasks.sim_goal states the schedule a student is being asked to produce —
// "these two jobs run at once, under six minutes, with this cache warm". It is
// the answer key of a sim task the same way check_script is one for a script
// task, and it travels through the admin response only.
//
// The scenario is the opposite and has to reach the student: without the
// catalogue of steps there is nothing to write a pipeline out of. Both halves
// are pinned here, because the failure that matters is somebody adding a goal
// field to make the editor easier and nobody noticing.
func TestLabDetailCarriesTheScenarioButNeverTheGoal(t *testing.T) {
	lab := &domain.Lab{
		ID: 11, Slug: "pipeline-dau-tien", Title: "Pipeline đầu tiên",
		SimScenario: []byte(`{"version":1,"runner_count":2,` +
			`"catalog":{"checkout":{"seconds":5},"npm-ci":{"seconds":90}}}`),
		Tasks: []domain.Task{{
			ID: 1, Title: "Cho test chạy song song với build", Points: 10,
			Kind: domain.KindSim,
		}},
	}

	b, err := json.Marshal(newLabDetail(lab))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(b)

	for _, leak := range []string{"sim_goal", "simGoal", "SimGoal", "jobs_parallel"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries %q:\n%s", leak, body)
		}
	}

	// Nested JSON, not a quoted string: the client reads the catalogue, and a
	// string of JSON would make it parse a field the server already parsed.
	if !strings.Contains(body, `"sim_scenario":{"version":1`) {
		t.Errorf("scenario should reach the student as JSON, got: %s", body)
	}
	if !strings.Contains(body, `"kind":"sim"`) {
		t.Errorf("kind should reach the student, got: %s", body)
	}
}

// A container lab has no scenario, and the field has to marshal as null rather
// than blowing up: an empty json.RawMessage is not valid JSON, and the whole
// response would fail over a column that is simply unset.
func TestLabDetailWithNoScenarioMarshalsNull(t *testing.T) {
	b, err := json.Marshal(newLabDetail(&domain.Lab{Slug: "lab-container"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"sim_scenario":null`) {
		t.Errorf(`want "sim_scenario":null, got: %s`, b)
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
