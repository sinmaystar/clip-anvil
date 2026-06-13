package auth

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

const AccountIDKey = "account_id"

func Middleware(secret string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		authHeader := string(c.GetHeader("Authorization"))
		tokenStr, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || tokenStr == "" {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			c.Abort()
			return
		}

		accountID, err := VerifyToken(tokenStr, secret)
		if err != nil {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			c.Abort()
			return
		}

		c.Set(AccountIDKey, accountID)
		c.Next(ctx)
	}
}
