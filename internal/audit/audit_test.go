package audit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// The point of this table is that it survives the accounts it talks about. An
// entry that turns into "someone did something to someone" the day an admin is
// deleted is not an audit log, and nothing else in the system would notice it
// had happened.
func TestEntriesOutliveTheAccountsTheyName(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("cần DATABASE_URL trỏ tới postgres đã migrate")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	r := New(db)
	actor, actorName := seedUser(t, db)
	target, targetName := seedUser(t, db)

	// TargetName left empty on purpose: resolving it is the recorder's job, and
	// a caller that had to look it up first would eventually forget to.
	err = r.Write(ctx, Entry{
		ActorID:    actor,
		Action:     ActionUserBan,
		TargetType: TargetUser,
		TargetID:   fmt.Sprint(target),
		Detail:     "spam",
		IP:         "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readEntry(t, db, actor)
	if got.ActorName != actorName || got.TargetName != targetName {
		t.Fatalf("entry = %+v, want names %q and %q resolved at write time",
			got, actorName, targetName)
	}
	if got.Detail != "spam" || got.IP != "10.0.0.1" {
		t.Fatalf("entry = %+v, want the reason and ip kept", got)
	}

	// Delete the admin who did it. The foreign key nulls the id; the snapshot is
	// the only thing left saying who, which is the whole reason it is stored.
	if err := db.Exec(`DELETE FROM users WHERE id = ?`, actor).Error; err != nil {
		t.Fatalf("delete actor: %v", err)
	}
	after := readEntryByID(t, db, got.ID)
	if after.ActorID != nil {
		t.Fatalf("actor_id = %v, want null after the account went", *after.ActorID)
	}
	if after.ActorName != actorName {
		t.Fatalf("actor_name = %q, want the snapshot %q to survive", after.ActorName, actorName)
	}

	// And the row is still readable through the API's own path.
	page, err := r.List(ctx, Filter{Action: ActionUserBan})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Limit != ListLimit || page.Total < 1 {
		t.Fatalf("page = total %d limit %d", page.Total, page.Limit)
	}
	found := false
	for _, l := range page.Logs {
		if l.ID == got.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("entry missing from the filtered list")
	}
}

func seedUser(t *testing.T, db *gorm.DB) (int64, string) {
	t.Helper()
	name := fmt.Sprintf("audit%d", time.Now().UnixNano())
	var id int64
	err := db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		name, name+"@test.local",
	).Scan(&id).Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM audit_logs WHERE actor_id = ? OR target_id = ?`, id, fmt.Sprint(id))
		db.Exec(`DELETE FROM users WHERE id = ?`, id)
	})
	return id, name
}

func readEntry(t *testing.T, db *gorm.DB, actor int64) Log {
	t.Helper()
	var l Log
	if err := db.Raw(
		`SELECT id, actor_id, actor_name, action, target_type, target_id,
		        target_name, detail, ip, created_at
		   FROM audit_logs WHERE actor_id = ? ORDER BY id DESC LIMIT 1`, actor,
	).Scan(&l).Error; err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if l.ID == 0 {
		t.Fatal("no entry written")
	}
	return l
}

func readEntryByID(t *testing.T, db *gorm.DB, id int64) Log {
	t.Helper()
	var l Log
	if err := db.Raw(
		`SELECT id, actor_id, actor_name, action, target_type, target_id,
		        target_name, detail, ip, created_at
		   FROM audit_logs WHERE id = ?`, id,
	).Scan(&l).Error; err != nil {
		t.Fatalf("read entry: %v", err)
	}
	// The cleanup keys off actor_id, which this test nulls out on purpose.
	t.Cleanup(func() { db.Exec(`DELETE FROM audit_logs WHERE id = ?`, id) })
	return l
}
