// auth/claims.go

package auth

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

func (c *Claims) UserID() string {
	if c == nil {
		return ""
	}
	return c.Subject
}

func (c *Claims) IsAdmin() bool { return c.HasRole("admin") }

func (c *Claims) HasRole(role string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}
