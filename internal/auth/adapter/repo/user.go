package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/domain"
)

type userModel struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash *string
	GoogleID     *string
	AvatarURL    *string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Roles        []string `gorm:"-"`
}

func (userModel) TableName() string { return "users" }

func toDomain(m *userModel) *domain.User {
	return &domain.User{
		ID:           m.ID,
		Username:     m.Username,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		GoogleID:     m.GoogleID,
		AvatarURL:    m.AvatarURL,
		Status:       domain.Status(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		Roles:        m.Roles,
	}
}

func fromDomain(u *domain.User) *userModel {
	return &userModel{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		GoogleID:     u.GoogleID,
		AvatarURL:    u.AvatarURL,
		Status:       string(u.Status),
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type UserRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *domain.User, role string) error {
	m := fromDomain(u)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = ?`,
			m.ID, role,
		).Error
	})
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	u.ID, u.CreatedAt, u.UpdatedAt = m.ID, m.CreatedAt, m.UpdatedAt
	return nil
}

func (r *UserRepo) get(ctx context.Context, where string, args ...any) (*domain.User, error) {
	var m userModel
	err := r.db.WithContext(ctx).Where(where, args...).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.loadRoles(ctx, &m); err != nil {
		return nil, err
	}
	return toDomain(&m), nil
}

func (r *UserRepo) ByID(ctx context.Context, id int64) (*domain.User, error) {
	return r.get(ctx, "id = ?", id)
}

func (r *UserRepo) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.get(ctx, "email = ?", email)
}

func (r *UserRepo) ByGoogleID(ctx context.Context, gid string) (*domain.User, error) {
	return r.get(ctx, "google_id = ?", gid)
}

func (r *UserRepo) ByLogin(ctx context.Context, login string) (*domain.User, error) {
	return r.get(ctx, "email = ? OR lower(username) = ?", login, login)
}

func (r *UserRepo) loadRoles(ctx context.Context, m *userModel) error {
	return r.db.WithContext(ctx).Raw(
		`SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ?`,
		m.ID,
	).Scan(&m.Roles).Error
}

func (r *UserRepo) UsernameTaken(ctx context.Context, name string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("users").Where("lower(username) = lower(?)", name).Count(&n).Error
	return n > 0, err
}

func (r *UserRepo) UpdateProfile(ctx context.Context, id int64, username, email string, avatarURL *string) error {
	fields := map[string]any{
		"username":   username,
		"email":      email,
		"updated_at": time.Now(),
	}
	if avatarURL != nil {
		if *avatarURL == "" {
			fields["avatar_url"] = nil
		} else {
			fields["avatar_url"] = *avatarURL
		}
	}
	err := r.db.WithContext(ctx).Table("users").Where("id = ?", id).Updates(fields).Error
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *UserRepo) UpdateAvatar(ctx context.Context, id int64, url string) error {
	return r.db.WithContext(ctx).Table("users").Where("id = ?", id).
		Updates(map[string]any{"avatar_url": url, "updated_at": time.Now()}).Error
}

func (r *UserRepo) SetStatus(ctx context.Context, id int64, status domain.Status) error {
	return r.db.WithContext(ctx).Table("users").Where("id = ?", id).
		Updates(map[string]any{"status": string(status), "updated_at": time.Now()}).Error
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return r.db.WithContext(ctx).Table("users").Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "updated_at": time.Now()}).Error
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE users SET banned_by = NULL WHERE banned_by = ?`, id).Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM users WHERE id = ?`, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *UserRepo) LinkGoogle(ctx context.Context, id int64, googleID, avatarURL string) error {
	return r.db.WithContext(ctx).Table("users").Where("id = ?", id).
		Updates(map[string]any{"google_id": googleID, "avatar_url": avatarURL, "updated_at": time.Now()}).Error
}
