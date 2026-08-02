package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/devforge/be/internal/auth/domain"
)

// The "active only" rule lives in the UPDATE's WHERE clause, and a ban that
// silently did nothing would report success to an admin watching the row not
// change. The unban half matters for the same reason in reverse: a banned_at
// left behind reads as banned to anything checking the timestamp rather than
// the status.
func TestBanUnbanAndRole(t *testing.T) {
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
	r := NewUserRepo(db)
	actor := seedUser(t, db, "active")
	target := seedUser(t, db, "active")
	pending := seedUser(t, db, "pending")

	if err := r.Ban(ctx, target, actor, "spam"); err != nil {
		t.Fatalf("ban: %v", err)
	}
	// Banning what is already banned is not the state the caller asked for
	// being reached anyway — it is a second admin overwriting the first one's
	// reason, which the WHERE clause is there to stop.
	if err := r.Ban(ctx, target, actor, "khác"); !errors.Is(err, domain.ErrNotActive) {
		t.Fatalf("re-ban = %v, want ErrNotActive", err)
	}
	if err := r.Ban(ctx, pending, actor, "x"); !errors.Is(err, domain.ErrNotActive) {
		t.Fatalf("ban pending = %v, want ErrNotActive", err)
	}
	if err := r.Ban(ctx, 0, actor, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ban missing user = %v, want ErrNotFound", err)
	}

	got := readUser(t, db, target)
	if got.Status != "banned" || got.BannedAt == nil || got.BannedReason == nil ||
		*got.BannedReason != "spam" {
		t.Fatalf("after ban = %+v, want banned with the first reason kept", got)
	}

	if err := r.Unban(ctx, target); err != nil {
		t.Fatalf("unban: %v", err)
	}
	got = readUser(t, db, target)
	if got.Status != "active" || got.BannedAt != nil || got.BannedReason != nil {
		t.Fatalf("after unban = %+v, want the whole ban trail cleared", got)
	}

	// Both directions are idempotent: an admin pressing twice asked for a state,
	// not for an error.
	if err := r.SetRole(ctx, target, domain.RoleAdmin, true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := r.SetRole(ctx, target, domain.RoleAdmin, true); err != nil {
		t.Fatalf("grant twice: %v", err)
	}
	if !hasRole(t, db, target, domain.RoleAdmin) {
		t.Fatal("role not granted")
	}
	if err := r.SetRole(ctx, target, domain.RoleAdmin, false); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := r.SetRole(ctx, target, domain.RoleAdmin, false); err != nil {
		t.Fatalf("revoke twice: %v", err)
	}
	if hasRole(t, db, target, domain.RoleAdmin) {
		t.Fatal("role not revoked")
	}
	if err := r.SetRole(ctx, 0, domain.RoleAdmin, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("role on missing user = %v, want ErrNotFound", err)
	}

	// The list carries the ban trail the table renders, so it is read back here
	// rather than trusted from the write above.
	if err := r.Ban(ctx, target, actor, "spam"); err != nil {
		t.Fatalf("re-ban for list: %v", err)
	}
	list, err := r.ListUsers(ctx, domain.UserFilter{Status: "banned"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var row *domain.ManagedUser
	for i := range list.Users {
		if list.Users[i].ID == target {
			row = &list.Users[i]
		}
	}
	if row == nil {
		t.Fatalf("banned user missing from status=banned list of %d", len(list.Users))
	}
	if row.BannedBy == nil {
		t.Fatal("banned_by not resolved to a username")
	}
	if list.Limit != domain.UserListLimit || list.Total < 1 {
		t.Fatalf("list meta = total %d limit %d", list.Total, list.Limit)
	}
}

// seedUser makes a throwaway account and removes it when the test ends.
func seedUser(t *testing.T, db *gorm.DB, status string) int64 {
	t.Helper()
	suffix := time.Now().UnixNano()
	var id int64
	err := db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', ?) RETURNING id`,
		fmt.Sprintf("admintest%d", suffix),
		fmt.Sprintf("admin-%d@test.local", suffix),
		status,
	).Scan(&id).Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`UPDATE users SET banned_by = NULL WHERE banned_by = ?`, id)
		db.Exec(`DELETE FROM users WHERE id = ?`, id)
	})
	return id
}

type userSnapshot struct {
	Status       string
	BannedAt     *time.Time
	BannedReason *string
}

func readUser(t *testing.T, db *gorm.DB, id int64) userSnapshot {
	t.Helper()
	var out userSnapshot
	if err := db.Raw(
		`SELECT status, banned_at, banned_reason FROM users WHERE id = ?`, id,
	).Scan(&out).Error; err != nil {
		t.Fatalf("read user: %v", err)
	}
	return out
}

func hasRole(t *testing.T, db *gorm.DB, id int64, role string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(
		`SELECT count(*) FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		  WHERE ur.user_id = ? AND r.name = ?`, id, role,
	).Scan(&n).Error; err != nil {
		t.Fatalf("read roles: %v", err)
	}
	return n > 0
}
