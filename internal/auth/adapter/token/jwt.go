package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.TokenIssuer = (*JWT)(nil)

// Refresh tokens are no longer signed blobs — they are session ids held in
// Redis — so "access" is the only type this ever mints or accepts.
const typeAccess = "access"

type claims struct {
	Type      string   `json:"typ"`
	Roles     []string `json:"roles,omitempty"`
	SessionID string   `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type JWT struct {
	secret    []byte
	accessTTL time.Duration
}

func NewJWT(secret string, accessTTL time.Duration) *JWT {
	return &JWT{secret: []byte(secret), accessTTL: accessTTL}
}

func (j *JWT) IssueAccess(userID int64, roles []string, sessionID string) (string, time.Duration, error) {
	access, err := j.sign(userID, typeAccess, roles, sessionID, j.accessTTL)
	if err != nil {
		return "", 0, err
	}
	return access, j.accessTTL, nil
}

func (j *JWT) ParseAccess(token string) (int64, []string, string, error) {
	c, err := j.parse(token, typeAccess)
	if err != nil {
		return 0, nil, "", err
	}
	id, err := userID(c)
	return id, c.Roles, c.SessionID, err
}

func (j *JWT) sign(uid int64, typ string, roles []string, sessionID string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := claims{
		Type:      typ,
		Roles:     roles,
		SessionID: sessionID,
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
