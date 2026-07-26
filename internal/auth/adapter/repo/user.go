package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.UserRepository = (*UserRepo)(nil)

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
	return r.get(ctx, "email = ? OR username = ?", login, login)
}

func (r *UserRepo) loadRoles(ctx context.Context, m *userModel) error {
	return r.db.WithContext(ctx).Raw(
		`SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ?`,
		m.ID,
	).Scan(&m.Roles).Error
}

func (r *UserRepo) UsernameTaken(ctx context.Context, name string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("users").Where("username = ?", name).Count(&n).Error
	return n > 0, err
}

func (r *UserRepo) LinkGoogle(ctx context.Context, id int64, googleID, avatarURL string) error {
	return r.db.WithContext(ctx).Table("users").Where("id = ?", id).
		Updates(map[string]any{"google_id": googleID, "avatar_url": avatarURL, "updated_at": time.Now()}).Error
}
