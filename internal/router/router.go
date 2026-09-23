package router

import (
	"warehouse-manager/internal/auth"
	"warehouse-manager/internal/middleware"

	"github.com/gin-gonic/gin"
)

type RouteRegistrar interface {
	RegisterRoutes(rg *gin.RouterGroup)
}

func NewRouter(authSvc *auth.Service, publicRegistrars, protectedRegistrars []RouteRegistrar) *gin.Engine {
	r := gin.Default()

	v1 := r.Group("/v1")

	public := v1.Group("")
	for _, reg := range publicRegistrars {
		reg.RegisterRoutes(public)
	}

	protected := v1.Group("")
	protected.Use(middleware.AuthRequired(authSvc))
	for _, reg := range protectedRegistrars {
		reg.RegisterRoutes(protected)
	}

	return r
}
