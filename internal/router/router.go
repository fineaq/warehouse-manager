package router

import "github.com/gin-gonic/gin"

type RouteRegistrar interface {
	RegisterRoutes(rg *gin.RouterGroup)
}

func NewRouter(registrars ...RouteRegistrar) *gin.Engine {
	r := gin.Default()

	v1 := r.Group("/v1")
	for _, reg := range registrars {
		reg.RegisterRoutes(v1)
	}

	return r
}
