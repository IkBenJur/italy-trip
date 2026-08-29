package users

import (
	"errors"
	"net/http"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/json"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	Queries repo.Querier
	Issuer  *auth.TokenIssuer
}

func NewHandler(queries repo.Querier, issuer *auth.TokenIssuer) *Handler {
	return &Handler{Queries: queries, Issuer: issuer}
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
