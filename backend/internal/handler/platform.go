package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/transport"
)

type PlatformHandler struct {
	Name string
}

func NewPlatformHandler(name string) *PlatformHandler {
	return &PlatformHandler{Name: name}
}

// Info returns platform branding info (public, no auth required).
func (h *PlatformHandler) Info(c *gin.Context) {
	transport.WriteData(c, http.StatusOK, map[string]string{
		"name": h.Name,
	})
}
