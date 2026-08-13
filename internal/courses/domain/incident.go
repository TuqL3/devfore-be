package domain

// Incident is one authored way of breaking a lab's service. A lab carries
// several and Start draws one at random, which is what makes replaying a drill a
// fresh problem rather than a memory test.
//
// Having an active row is the whole of what makes a lab a War Room drill — there
// is no flag column saying so. That is why the admin screen shows the count on
// the lab: it is the only way to tell a drill from an ordinary lab.
//
// Unlike Task/AdminTask there is no student-facing twin of this type. Every
// field here is either the answer key (BreakScript), the answer itself (Title,
// RevealMD, read only once the attempt is over) or an authoring detail — so
// nothing of it may travel on a public response, and there is no shape it could
// safely take if it did.
type Incident struct {
	ID    int64
	Title string
	// Runs inside the student's container to cause the fault. Same trust level as
	// lab_tasks.check_script: authored by an admin, never sent to a client.
	BreakScript string
	// Read after the drill: what had happened and how it is normally found.
	RevealMD string
	// Requests per second the outage is assumed to hurt. An authored number, not
	// a measurement — every screen that shows it says so.
	RPS int
	// Retiring is a flag rather than a delete because finished sessions point at
	// the row that broke them and their reports still read it back.
	Active bool
}

// Drill is a lab seen from the War Room admin list: enough to recognise it and
// to reach its scenarios, plus the course it is filed under.
//
// Its own type rather than domain.Lab because the two answer different
// questions — Lab is "what is in this course", Drill is "what does War Room
// offer" — and the description, task count and scenario JSON that Lab carries
// are all dead weight on a list nobody reads a lab body from.
type Drill struct {
	ID              int64
	Slug            string
	Title           string
	DurationMinutes int
	LabImageID      *int64
	IncidentSetup   string
	// nil for a challenge created in War Room, which belongs to no course. Set
	// only on the older drills that were made inside one before 000028.
	CourseID    *int64
	CourseTitle string
	// draft or published, held apart from whether any scenario is active: one
	// says whether the challenge is offered at all, the other which faults can
	// be drawn once it is.
	Status string
	// Scenarios that can still be drawn. Zero means the lab has fallen out of
	// War Room without being deleted, which is the state this list exists to
	// make visible.
	IncidentCount int
	// Every scenario including the retired ones, so a lab with nothing active
	// still says how much work is already in it.
	ScenarioCount int
}

type IncidentInput struct {
	Title       string
	BreakScript string
	RevealMD    string
	RPS         int
	Active      bool
}
