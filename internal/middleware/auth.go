package middleware

import (
	"strings"
	"warehouse-manager/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func AuthRequired(authSvc *auth.Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")

		if token == "" {
			ctx.AbortWithStatusJSON(401, gin.H{"error": "missing token"})
			return
		}

		claims, err := authSvc.ValidateToken(token)

		if err != nil {
			ctx.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}

		userID, err := uuid.Parse(claims.Subject)

		if err != nil {
			ctx.AbortWithStatusJSON(401, gin.H{"error": "invalid token subject"})
			return
		}

		ctx.Set("tenant_id", claims.TenantID)
		ctx.Set("user_id", userID)
		ctx.Next()
	}
}
