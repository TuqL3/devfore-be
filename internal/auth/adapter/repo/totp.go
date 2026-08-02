package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/domain"
)

// TOTPSecret reads the enrolment row. Returns ErrNotFound when there is none,
// which is the ordinary state for most accounts rather than a problem.
func (r *UserRepo) TOTPSecret(ctx context.Context, userID int64) (*domain.TOTP, error) {
	var row struct {
		Secret      string
		ConfirmedAt *time.Time
	}
	res := r.db.WithContext(ctx).Raw(
		`SELECT secret, confirmed_at FROM user_totp WHERE user_id = ?`, userID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return &domain.TOTP{Secret: row.Secret, ConfirmedAt: row.ConfirmedAt}, nil
}

// StartTOTP writes an unconfirmed secret, replacing any enrolment in progress.
// Refuses to touch a confirmed one: overwriting that would let anyone holding a
// live session quietly move the second factor to their own phone.
func (r *UserRepo) StartTOTP(ctx context.Context, userID int64, secret string) error {
	res := r.db.WithContext(ctx).Exec(
		`INSERT INTO user_totp (user_id, secret) VALUES (?, ?)
		 ON CONFLICT (user_id) DO UPDATE
		 SET secret = EXCLUDED.secret, created_at = now()
		 WHERE user_totp.confirmed_at IS NULL`,
		userID, secret,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTOTPEnabled
	}
	return nil
}

// ConfirmTOTP marks the enrolment done and stores the recovery codes in the same
// transaction. One without the other is the failure worth avoiding: a confirmed
// factor with no recovery codes is an account one lost phone away from gone.
func (r *UserRepo) ConfirmTOTP(ctx context.Context, userID int64, codeHashes []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(
			`UPDATE user_totp SET confirmed_at = now()
			  WHERE user_id = ? AND confirmed_at IS NULL`, userID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		// Any codes from an earlier enrolment are dead the moment a new secret
		// is confirmed.
		if err := tx.Exec(`DELETE FROM user_recovery_codes WHERE user_id = ?`, userID).Error; err != nil {
			return err
		}
		for _, h := range codeHashes {
			if err := tx.Exec(
				`INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, ?)`,
				userID, h,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DisableTOTP removes the factor and the codes with it.
func (r *UserRepo) DisableTOTP(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM user_recovery_codes WHERE user_id = ?`, userID).Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM user_totp WHERE user_id = ?`, userID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// UnusedRecoveryCodes returns the hashes still worth checking a submission
// against, with their ids so the one that matches can be spent.
func (r *UserRepo) UnusedRecoveryCodes(ctx context.Context, userID int64) ([]domain.RecoveryCode, error) {
	rows := []domain.RecoveryCode{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, code_hash FROM user_recovery_codes
		  WHERE user_id = ? AND used_at IS NULL ORDER BY id`, userID,
	).Scan(&rows).Error
	return rows, err
}

// SpendRecoveryCode marks one used. The WHERE clause carries the "still unused"
// condition rather than a read before it: two logins racing on the same code
// must not both count, and the one that changes no rows is the one that lost.
func (r *UserRepo) SpendRecoveryCode(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE user_recovery_codes SET used_at = now()
		  WHERE id = ? AND used_at IS NULL`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrInvalidCode
	}
	return nil
}

// CountUnusedRecoveryCodes is what the security screen shows so somebody down to
// their last code finds out before they need it.
func (r *UserRepo) CountUnusedRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		userID,
	).Scan(&n).Error
	return n, err
}
