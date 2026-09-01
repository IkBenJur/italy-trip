package users

import (
	"errors"
	"net/http"
	"time"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// refreshCookieName is scoped to /auth so it isn't sent along with every other
// request - only login and refresh ever need to read it.
const refreshCookieName = "refresh_token"
const refreshCookiePath = "/auth"

type Handler struct {
	Queries         repo.Querier
	Issuer          *auth.TokenIssuer
	RefreshTokenTTL time.Duration
}

func NewHandler(queries repo.Querier, issuer *auth.TokenIssuer, refreshTokenTTL time.Duration) *Handler {
	return &Handler{Queries: queries, Issuer: issuer, RefreshTokenTTL: refreshTokenTTL}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type authResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func toUserResponse(user repo.User) userResponse {
	return userResponse{
		ID:    utils.UUIDString(user.ID),
		Email: user.Email,
	}
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		json.WriteError(c, http.StatusBadRequest, err)
		return
	}

	user, err := h.Queries.FindUserByEmail(c, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			json.WriteErrorFromString(c, http.StatusUnauthorized, "invalid email or password")
			return
		}
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to login", err)
		return
	}

	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		json.WriteErrorFromString(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := h.Issuer.Issue(utils.UUIDString(user.ID))
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to issue token", err)
		return
	}

	if err := h.issueRefreshToken(c, user.ID); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to issue refresh token", err)
		return
	}

	json.WriteJSON(c, http.StatusOK, authResponse{Token: token, User: toUserResponse(user)})
}

// Refresh exchanges the refresh-token cookie for a new access token, rotating
// the refresh token in the process. It sits outside RequireAuth: its whole
// point is to work once the access token has already expired.
func (h *Handler) Refresh(c *gin.Context) {
	rawToken, err := c.Cookie(refreshCookieName)
	if err != nil || rawToken == "" {
		json.WriteErrorFromString(c, http.StatusUnauthorized, "missing refresh token")
		return
	}

	stored, err := h.Queries.FindRefreshTokenByHash(c, auth.HashRefreshToken(rawToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			clearRefreshCookie(c)
			json.WriteErrorFromString(c, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to look up refresh token", err)
		return
	}

	// A revoked token being presented again means it was already rotated and
	// this is a second, unexpected use - the classic sign of a stolen token.
	// Revoke the whole family rather than trust just this one request.
	if stored.RevokedAt.Valid {
		_ = h.Queries.RevokeRefreshTokensForUser(c, stored.UserID)
		clearRefreshCookie(c)
		json.WriteErrorFromString(c, http.StatusUnauthorized, "refresh token already used")
		return
	}

	if time.Now().After(stored.ExpiresAt.Time) {
		clearRefreshCookie(c)
		json.WriteErrorFromString(c, http.StatusUnauthorized, "refresh token expired")
		return
	}

	if err := h.Queries.RevokeRefreshToken(c, stored.ID); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to revoke refresh token", err)
		return
	}

	user, err := h.Queries.FindUserById(c, stored.UserID)
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusUnauthorized, "user not found", err)
		return
	}

	token, err := h.Issuer.Issue(utils.UUIDString(user.ID))
	if err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to issue token", err)
		return
	}

	if err := h.issueRefreshToken(c, user.ID); err != nil {
		json.WriteErrorFromStringWithErrorObjectLog(c, http.StatusInternalServerError, "failed to issue refresh token", err)
		return
	}

	json.WriteJSON(c, http.StatusOK, authResponse{Token: token, User: toUserResponse(user)})
}

func (h *Handler) Me(c *gin.Context) {
	user, ok := middleware.UserFromContext(c)
	if !ok {
		json.WriteErrorFromString(c, http.StatusUnauthorized, "not authenticated")
		return
	}

	json.WriteJSON(c, http.StatusOK, toUserResponse(user))
}

func (h *Handler) issueRefreshToken(c *gin.Context, userID pgtype.UUID) error {
	rawToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return err
	}

	_, err = h.Queries.CreateRefreshToken(c, repo.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: auth.HashRefreshToken(rawToken),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.RefreshTokenTTL), Valid: true},
	})
	if err != nil {
		return err
	}

	setRefreshCookie(c, rawToken, h.RefreshTokenTTL)
	return nil
}

// setRefreshCookie and clearRefreshCookie use SameSite=None+Secure because the
// frontend and API are on different origins even in local dev (localhost:5173
// vs localhost:8080). Browsers treat http://localhost as a secure context, so
// this also works without HTTPS locally.
func setRefreshCookie(c *gin.Context, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(refreshCookieName, token, int(ttl.Seconds()), refreshCookiePath, "", true, true)
}

func clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", true, true)
}
