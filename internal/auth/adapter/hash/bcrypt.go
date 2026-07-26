package hash

import (
	"golang.org/x/crypto/bcrypt"

	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.PasswordHasher = Bcrypt{}

type Bcrypt struct{}

func (Bcrypt) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	return string(b), err
}

func (Bcrypt) Check(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
