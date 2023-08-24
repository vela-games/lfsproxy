package handlers

import "github.com/gin-gonic/gin"

type HealthHandler struct{}

func (h HealthHandler) Get(c *gin.Context) {
	c.AbortWithStatusJSON(200, gin.H{
		"health": "ok",
	})
}
