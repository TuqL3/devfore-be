package domain

import "time"

// ManagedUser is one row of the admin user table. Separate from User because it
// carries the ban trail and deliberately does not carry the password hash or the
// Google id: this type is serialised straight to an admin screen, and neither
// belongs in a response.
type ManagedUser struct {
	ID        int64
	Username  string
	Email     string
	AvatarURL *string
	Status    Status
	Roles     []string
	CreatedAt time.Time
	// Set only while Status is banned. Kept after an unban would be a lie: the
	// row would read as banned to anything checking the timestamp.
	BannedAt     *time.Time
	BannedReason *string
	// Who did it. Nil for a ban by an account that has since been deleted —
	// users.banned_by is nulled out rather than cascading the ban away.
	BannedBy *string
}

// UserFilter is what the admin list screen can narrow by. Every field is
// optional and an empty one means "no filter" rather than "match empty".
type UserFilter struct {
	Query  string
	Status string
	Role   string
}

// UserListLimit caps one page of the admin user list.
//
// ponytail: a hard cap and a total, not pagination. The screen says how many it
// is not showing, which is honest and costs three lines; cursor paging is the
// upgrade when an install outgrows a search box.
const UserListLimit = 200

// ManagedUsers is a capped page together with the size of the full match, so the
// screen can say what it left out instead of quietly truncating.
type ManagedUsers struct {
	Users []ManagedUser
	Total int
	Limit int
}
