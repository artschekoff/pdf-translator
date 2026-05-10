package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	CtxUserID = "user_id"
	CtxEmail  = "email"
	CtxRoles  = "roles"
)

type Claims struct {
	jwt.RegisteredClaims
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	Scopes      []string `json:"scopes"`
	AuthMethod  string   `json:"auth_method"`
}

func Middleware(jwks *JWKS) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		raw := strings.TrimPrefix(header, "Bearer ")

		token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			kid, _ := t.Header["kid"].(string)
			return jwks.GetKey(kid)
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims := token.Claims.(*Claims)
		c.Set(CtxUserID, claims.Subject)
		c.Set(CtxEmail, claims.Email)
		c.Set(CtxRoles, claims.Roles)
		c.Next()
	}
}

func UserID(c *gin.Context) string {
	id, _ := c.Get(CtxUserID)
	s, _ := id.(string)
	return s
}
