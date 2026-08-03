package repo

import (
	"context"
	"strings"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type managedRow struct {
	ID           int64
	Username     string
	Email        string
	AvatarURL    *string
	Status       string
	Roles        string
	CreatedAt    time.Time
	BannedAt     *time.Time
	BannedReason *string
	BannedBy     *string
}

// ListUsers reads one capped page of the admin table together with the size of
// the full match. Two queries against the same WHERE clause, built once here so
// the count and the page cannot drift apart.
//
// Roles arrive as a comma-joined string rather than a second query per row: the
// alternative is either N+1 or an array scan gorm has to be told about, and the
// join is already here for the role filter.
func (r *UserRepo) ListUsers(
	ctx context.Context, f domain.UserFilter,
) (*domain.ManagedUsers, error) {
	where := []string{"1 = 1"}
	args := []any{}

	if q := strings.TrimSpace(f.Query); q != "" {
		// ILIKE, so an admin looking for a student does not have to know how
		// they capitalised their own name.
		where = append(where, "(u.username ILIKE ? OR u.email ILIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if f.Status != "" {
		where = append(where, "u.status = ?")
		args = append(args, f.Status)
	}
	if f.Role != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM user_roles ur JOIN roles rr ON rr.id = ur.role_id
			 WHERE ur.user_id = u.id AND rr.name = ?)`)
		args = append(args, f.Role)
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM users u WHERE `+clause, args...,
	).Scan(&total).Error; err != nil {
		return nil, err
	}

	rows := []managedRow{}
	// Banned first, then newest: the rows an admin opened this screen for are
	// the ones that need a decision, not the ones that registered last week.
	if err := r.db.WithContext(ctx).Raw(
		`SELECT u.id, u.username, u.email, u.avatar_url, u.status, u.created_at,
		        u.banned_at, u.banned_reason,
		        b.username AS banned_by,
		        COALESCE((
		          SELECT string_agg(rr.name, ',' ORDER BY rr.name)
		            FROM user_roles ur JOIN roles rr ON rr.id = ur.role_id
		           WHERE ur.user_id = u.id
		        ), '') AS roles
		   FROM users u
		   LEFT JOIN users b ON b.id = u.banned_by
		  WHERE `+clause+`
		  ORDER BY (u.status = 'banned') DESC, u.created_at DESC
		  LIMIT ?`, append(args, domain.UserListLimit)...,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := domain.ManagedUsers{
		Users: make([]domain.ManagedUser, len(rows)),
		Total: total,
		Limit: domain.UserListLimit,
	}
	for i, row := range rows {
		var roles []string
		if row.Roles != "" {
			roles = strings.Split(row.Roles, ",")
		}
		out.Users[i] = domain.ManagedUser{
			ID:           row.ID,
			Username:     row.Username,
			Email:        row.Email,
			AvatarURL:    row.AvatarURL,
			Status:       domain.Status(row.Status),
			Roles:        roles,
			CreatedAt:    row.CreatedAt,
			BannedAt:     row.BannedAt,
			BannedReason: row.BannedReason,
			BannedBy:     row.BannedBy,
		}
	}
	return &out, nil
}

// Ban records the decision and who made it in one write. The WHERE clause
// carries the active check rather than a read before it: two admins pressing the
// button together must not both count as the ban, and the second would otherwise
// overwrite the first one's reason.
func (r *UserRepo) Ban(ctx context.Context, id, by int64, reason string) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE users
		    SET status = 'banned', banned_at = now(), banned_by = ?,
		        banned_reason = NULLIF(?, ''), updated_at = now()
		  WHERE id = ? AND status = 'active'`,
		by, reason, id,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return r.whyNotBanned(ctx, id)
	}
	return nil
}

// whyNotBanned turns an update that changed nothing into the reason it changed
// nothing. Only reached on that path, so the extra read costs nothing in the
// case that works.
func (r *UserRepo) whyNotBanned(ctx context.Context, id int64) error {
	var status string
	res := r.db.WithContext(ctx).Raw(`SELECT status FROM users WHERE id = ?`, id).Scan(&status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return domain.ErrNotActive
}

// Unban clears the whole trail, not just the status. A banned_at left behind on
// an active account reads as banned to anything that checks the timestamp
// instead of the status.
func (r *UserRepo) Unban(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE users
		    SET status = 'active', banned_at = NULL, banned_by = NULL,
		        banned_reason = NULL, updated_at = now()
		  WHERE id = ? AND status = 'banned'`, id,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Either no such user or already active. Both leave the caller with an
		// active account, which is what they asked for.
		return r.exists(ctx, id)
	}
	return nil
}

func (r *UserRepo) exists(ctx context.Context, id int64) error {
	var n int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM users WHERE id = ?`, id).
		Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetRole grants or revokes one role. Idempotent both ways: granting a role
// already held and revoking one not held are both the state the caller asked
// for, and neither is worth an error.
func (r *UserRepo) SetRole(ctx context.Context, id int64, role string, on bool) error {
	if err := r.exists(ctx, id); err != nil {
		return err
	}
	if on {
		return r.db.WithContext(ctx).Exec(
			`INSERT INTO user_roles (user_id, role_id)
			 SELECT ?, id FROM roles WHERE name = ?
			 ON CONFLICT DO NOTHING`, id, role,
		).Error
	}
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM user_roles ur USING roles r
		  WHERE ur.role_id = r.id AND ur.user_id = ? AND r.name = ?`, id, role,
	).Error
}
