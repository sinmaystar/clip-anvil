package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const timeFormatRFC3339 = time.RFC3339Nano

type AuthHandler struct {
	queries     *db.Queries
	jwtSecret   string
	expireHours int
}

func NewAuthHandler(queries *db.Queries, jwtSecret string, expireHours int) *AuthHandler {
	return &AuthHandler{
		queries:     queries,
		jwtSecret:   jwtSecret,
		expireHours: expireHours,
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type accountResponse struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type authResponse struct {
	Token   string          `json:"token"`
	Account accountResponse `json:"account"`
}

func (h *AuthHandler) Register(ctx context.Context, c *app.RequestContext) {
	var req registerRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !validRegisterRequest(req) {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to hash password")
		return
	}

	account, err := h.queries.CreateAccount(ctx, db.CreateAccountParams{
		Email:        strings.TrimSpace(req.Email),
		PasswordHash: passwordHash,
		Name:         strings.TrimSpace(req.Name),
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(c, consts.StatusConflict, "email already registered")
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to create account")
		return
	}

	h.writeAuthResponse(c, account)
}

func (h *AuthHandler) Login(ctx context.Context, c *app.RequestContext) {
	var req loginRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !validEmail(req.Email) || req.Password == "" {
		writeError(c, consts.StatusUnauthorized, "invalid email or password")
		return
	}

	account, err := h.queries.GetAccountByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to load account")
		return
	}
	if !auth.CheckPassword(req.Password, account.PasswordHash) {
		writeError(c, consts.StatusUnauthorized, "invalid email or password")
		return
	}

	h.writeAuthResponse(c, account)
}

func (h *AuthHandler) Me(ctx context.Context, c *app.RequestContext) {
	value, ok := c.Get(auth.AccountIDKey)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	accountID, ok := value.(pgtype.UUID)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	account, err := h.queries.GetAccountByID(ctx, accountID)
	if err != nil {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	c.JSON(consts.StatusOK, toAccountResponse(account))
}

func (h *AuthHandler) writeAuthResponse(c *app.RequestContext, account db.Account) {
	token, err := auth.SignToken(account.ID, h.jwtSecret, h.expireHours)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to sign token")
		return
	}

	c.JSON(consts.StatusOK, authResponse{
		Token:   token,
		Account: toAccountResponse(account),
	})
}

func validRegisterRequest(req registerRequest) bool {
	return validEmail(req.Email) && len(req.Password) >= 6 && strings.TrimSpace(req.Name) != ""
}

func validEmail(email string) bool {
	email = strings.TrimSpace(email)
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func toAccountResponse(account db.Account) accountResponse {
	response := accountResponse{
		ID:    uuidToString(account.ID),
		Email: account.Email,
		Name:  account.Name,
	}
	if account.AvatarUrl.Valid {
		response.AvatarURL = &account.AvatarUrl.String
	}
	return response
}

func uuidToString(id pgtype.UUID) string {
	return strings.ToLower(id.String())
}

func writeError(c *app.RequestContext, status int, message string) {
	c.JSON(status, map[string]string{"error": message})
}
