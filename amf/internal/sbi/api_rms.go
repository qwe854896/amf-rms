package sbi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) getRMSRoutes() []Route {
	return []Route{
		{
			Name:    "root",
			Method:  http.MethodGet,
			Pattern: "/",
			APIFunc: func(c *gin.Context) {
				c.String(http.StatusOK, "Hello World!")
			},
		},
		// add more Route based on provided spec
	}
}
