package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/json"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
)

type contextKey string

const UserKey contextKey = "user"

// RequireAuth verifies the bearer token, loads the referenced user, and
// stores it in the gin context for downstream handlers to read via UserFromContext.
func RequireAuth(issuer *auth.TokenIssuer, queries repo.Querier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			json.WriteErrorFromString(c, http.StatusUnauthorized, "missing or malformed authorization header")
			c.Abort()
			return
		}

		claims, err := issuer.Verify(parts[1])
		if err != nil {
			if errors.Is(err, auth.ErrExpiredToken) {
				// "token_expired" tells the client a refresh can fix this, as
				// opposed to a token that will never be valid again.
				json.WriteErrorWithCode(c, http.StatusUnauthorized, "token expired", "token_expired")
			} else {
				json.WriteErrorFromString(c, http.StatusUnauthorized, "invalid token")
			}
			c.Abort()
			return
		}

		userID, err := utils.ParseUUID(claims.UserID)
		if err != nil {
			json.WriteErrorFromString(c, http.StatusUnauthorized, "invalid token subject")
			c.Abort()
			return
		}

		user, err := queries.FindUserById(c, userID)
		if err != nil {
			json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusUnauthorized, "user not found", err)
			c.Abort()
			return
		}

		c.Set(string(UserKey), user)
		c.Next()
	}
}

func UserFromContext(c *gin.Context) (repo.User, bool) {
	val, exists := c.Get(string(UserKey))
	if !exists {
		return repo.User{}, false
	}
	user, ok := val.(repo.User)
	return user, ok
}
