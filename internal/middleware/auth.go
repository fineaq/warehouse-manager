package middleware

import (
	"strings"
	"warehouse-manager/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ctxUserID   = "user_id"
	ctxTenantID = "tenant_id"
)

func AuthRequired(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")

		if token == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing token"})
			return
		}

		claims, err := authSvc.ValidateToken(token)

		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}

		userID, err := uuid.Parse(claims.Subject)

		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}

		tenantID, err := uuid.Parse(claims.TenantID)

		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}

		c.Set(ctxUserID, userID)
		c.Set(ctxTenantID, tenantID)
		c.Next()
	}
}

func UserID(c *gin.Context) (uuid.UUID, bool) {
	s, ok := c.Get(ctxUserID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := s.(uuid.UUID)
	return id, ok
}

func TenantID(c *gin.Context) (uuid.UUID, bool) {
	s, ok := c.Get(ctxTenantID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := s.(uuid.UUID)
	return id, ok
}
