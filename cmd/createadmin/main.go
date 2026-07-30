// Command createadmin makes or promotes the account that reaches the admin
// screens. It exists as a command rather than a seed row because a password
// belongs to whoever runs it: a hash committed to the repository is a shared
// credential, and the first person to clone the project would own it.
//
//	go run ./cmd/createadmin -email you@example.com -username admin -password '...'
//
// Running it again for an existing email promotes that account instead of
// failing, which is how a normal account becomes an admin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/adapter/hash"
	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "email của tài khoản admin")
	username := flag.String("username", "", "tên đăng nhập (mặc định lấy phần trước @)")
	// Read from the environment as well so a password does not have to sit in
	// shell history to be used once.
	password := flag.String("password", os.Getenv("ADMIN_PASSWORD"), "mật khẩu (hoặc đặt ADMIN_PASSWORD)")
	flag.Parse()

	*email = strings.ToLower(strings.TrimSpace(*email))
	if *email == "" || *password == "" {
		flag.Usage()
		return errors.New("cần -email và -password")
	}
	if len(*password) < 8 {
		return errors.New("mật khẩu phải từ 8 ký tự")
	}
	if *username == "" {
		*username, _, _ = strings.Cut(*email, "@")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Quiet logging on purpose: the verbose mode echoes every statement, and one
	// of the statements below carries the password hash.
	gdb, err := db.Open(cfg.DSN(), true)
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	created, err := upsertAdmin(context.Background(), gdb, *email, *username, *password)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("đã tạo admin %s (%s)\n", *username, *email)
	} else {
		fmt.Printf("tài khoản %s đã tồn tại — đã cấp quyền admin và đặt lại mật khẩu\n", *email)
	}
	return nil
}

// upsertAdmin writes the account and the role together. Splitting them would
// leave, on a failure between the two, an account that looks ordinary and a
// password its owner believes grants admin.
func upsertAdmin(ctx context.Context, gdb *gorm.DB, email, username, password string) (created bool, err error) {
	hashed, err := hash.Bcrypt{}.Hash(password)
	if err != nil {
		return false, err
	}

	err = gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id int64
		res := tx.Raw(`SELECT id FROM users WHERE lower(email) = ?`, email).Scan(&id)
		if res.Error != nil {
			return res.Error
		}
		created = res.RowsAffected == 0

		if created {
			if err := tx.Raw(
				`INSERT INTO users (username, email, password_hash, status)
				 VALUES (?, ?, ?, 'active') RETURNING id`,
				username, email, hashed,
			).Scan(&id).Error; err != nil {
				return err
			}
		} else if err := tx.Exec(
			// An existing account keeps its username: it may already be on a
			// leaderboard, and this command is not a rename tool.
			`UPDATE users SET password_hash = ?, status = 'active', updated_at = now()
			  WHERE id = ?`, hashed, id,
		).Error; err != nil {
			return err
		}

		return tx.Exec(
			`INSERT INTO user_roles (user_id, role_id)
			 SELECT ?, id FROM roles WHERE name = 'admin'
			 ON CONFLICT DO NOTHING`, id,
		).Error
	})
	return created, err
}
