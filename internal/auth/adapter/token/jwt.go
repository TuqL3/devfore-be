package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.TokenIssuer = (*JWT)(nil)

const (
	typeAccess  = "access"
	typeRefresh = "refresh"
)

type claims struct {
	Type  string   `json:"typ"`
	Roles []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

type JWT struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewJWT(secret string, accessTTL, refreshTTL time.Duration) *JWT {
	return &JWT{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (j *JWT) Issue(userID int64, roles []string) (domain.TokenPair, error) {
	access, err := j.sign(userID, typeAccess, roles, j.accessTTL)
	if err != nil {
		return domain.TokenPair{}, err
	}
	refresh, err := j.sign(userID, typeRefresh, nil, j.refreshTTL)
	if err != nil {
		return domain.TokenPair{}, err
	}
	return domain.TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(j.accessTTL.Seconds()),
	}, nil
}

func (j *JWT) ParseAccess(token string) (int64, []string, error) {
	c, err := j.parse(token, typeAccess)
	if err != nil {
		return 0, nil, err
	}
	id, err := userID(c)
	return id, c.Roles, err
}

func (j *JWT) ParseRefresh(token string) (int64, error) {
	c, err := j.parse(token, typeRefresh)
	if err != nil {
		return 0, err
	}
	return userID(c)
}

func (j *JWT) sign(uid int64, typ string, roles []string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := claims{
		Type:  typ,
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", uid),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(j.secret)
}

func (j *JWT) parse(token, wantType string) (*claims, error) {
	c := &claims{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, domain.ErrInvalidToken
		}
		return j.secret, nil
	})
	if err != nil || c.Type != wantType {
		return nil, domain.ErrInvalidToken
	}
	return c, nil
}

func userID(c *claims) (int64, error) {
	var id int64
	if _, err := fmt.Sscan(c.Subject, &id); err != nil {
		return 0, domain.ErrInvalidToken
	}
	return id, nil
}
