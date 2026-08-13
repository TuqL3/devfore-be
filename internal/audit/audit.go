// Package audit records the moderation actions an admin takes on somebody
// else's account or container.
//
// Its own package rather than a table inside auth or labs, because both modules
// write to it and neither should have to import the other to do so.
package audit

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// The actions worth a row. Named constants rather than strings at the call site:
// a typo in one of these produces a log line that no filter will ever find
// again, and nothing else would catch it.
const (
	ActionUserBan     = "user.ban"
	ActionUserUnban   = "user.unban"
	ActionRoleGrant   = "user.role_grant"
	ActionRoleRevoke  = "user.role_revoke"
	ActionSessionKill = "lab_session.kill"
	// Gỡ một báo cáo ai đó đã đăng công khai. Ghi lại vì nó là hành động lên nội
	// dung của người khác — thứ luôn phải có dấu vết ai làm, lúc nào.
	ActionDrillUnshare = "drill.unshare"
	// Đọc hoạt động của một người, và đọc lịch sử lệnh của một ca trực. Ghi lại
	// vì đây là đọc **việc của người khác** — pipeline họ viết, lệnh họ gõ — chứ
	// không phải đọc một con số của hệ thống.
	ActionUserActivityRead = "user.activity_read"
	ActionCommandLogRead   = "lab_session.command_log_read"
	// Xoá tin nhắn của người khác trong phòng chung.
	ActionChatDelete  = "chat.delete"
	ActionTOTPEnable  = "auth.totp_enable"
	ActionTOTPDisable = "auth.totp_disable"
)

const (
	TargetUser        = "user"
	TargetSession     = "lab_session"
	TargetChatMessage = "chat_message"
)

// Entry is one action as the caller describes it. ActorName and TargetName are
// filled in here rather than by the caller: a handler holds ids, and the name
// that belongs in the log is the one true at the time of writing.
type Entry struct {
	ActorID    int64
	Action     string
	TargetType string
	TargetID   string
	// Optional. Resolved from users when TargetType is TargetUser and this is
	// empty.
	//
	// ponytail: only user targets get resolved. A killed session shows its id,
	// which is enough to find the row it belonged to; joining lab_sessions here
	// would mean this package knowing another module's schema.
	TargetName string
	Detail     string
	IP         string
}

type Log struct {
	ID         int64
	ActorID    *int64
	ActorName  string
	Action     string
	TargetType string
	TargetID   string
	TargetName string
	Detail     string
	IP         string
	CreatedAt  time.Time
}

type Filter struct {
	Action string
	Actor  string
}

// ListLimit caps one read of the log.
//
// ponytail: a cap and a total, same as the user list. Paging an audit log is a
// real feature the day somebody has to answer a question with it; until then a
// screen that says what it is not showing is the honest version.
const ListLimit = 200

type Page struct {
	Logs  []Log
	Total int
	Limit int
}

type Recorder struct{ db *gorm.DB }

func New(db *gorm.DB) *Recorder { return &Recorder{db: db} }

// Write records one action. It is called after the action succeeded, so an error
// here means the thing happened and the record of it did not — which is why the
// caller is told to log and carry on rather than to fail the request. Reporting
// a ban as failed when the account is banned would be the worse lie of the two.
func (r *Recorder) Write(ctx context.Context, e Entry) error {
	var actorName string
	if err := r.db.WithContext(ctx).Raw(
		`SELECT username FROM users WHERE id = ?`, e.ActorID,
	).Scan(&actorName).Error; err != nil {
		return fmt.Errorf("audit actor: %w", err)
	}
	if actorName == "" {
		// Nothing sensible to snapshot, and an empty actor column would read as
		// a system action. Say what it was instead.
		actorName = fmt.Sprintf("#%d", e.ActorID)
	}

	name := e.TargetName
	if name == "" && e.TargetType == TargetUser {
		if err := r.db.WithContext(ctx).Raw(
			`SELECT username FROM users WHERE id::text = ?`, e.TargetID,
		).Scan(&name).Error; err != nil {
			return fmt.Errorf("audit target: %w", err)
		}
	}

	return r.db.WithContext(ctx).Exec(
		`INSERT INTO audit_logs
		   (actor_id, actor_name, action, target_type, target_id, target_name, detail, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ActorID, actorName, e.Action, e.TargetType, e.TargetID, name, e.Detail, e.IP,
	).Error
}

// List reads the log newest first.
func (r *Recorder) List(ctx context.Context, f Filter) (*Page, error) {
	where := "1 = 1"
	args := []any{}
	if f.Action != "" {
		where += " AND action = ?"
		args = append(args, f.Action)
	}
	if f.Actor != "" {
		where += " AND actor_name ILIKE ?"
		args = append(args, "%"+f.Actor+"%")
	}

	var total int
	if err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM audit_logs WHERE `+where, args...,
	).Scan(&total).Error; err != nil {
		return nil, err
	}

	logs := []Log{}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, actor_id, actor_name, action, target_type, target_id,
		        target_name, detail, ip, created_at
		   FROM audit_logs WHERE `+where+`
		  ORDER BY created_at DESC, id DESC
		  LIMIT ?`, append(args, ListLimit)...,
	).Scan(&logs).Error; err != nil {
		return nil, err
	}
	return &Page{Logs: logs, Total: total, Limit: ListLimit}, nil
}
