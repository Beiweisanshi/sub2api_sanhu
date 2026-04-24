// mkx: 新增模型广场用户接口，2026-04-24
package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type PlazaHandler struct {
	plazaSvc *service.ModelPlazaService
}

func NewPlazaHandler(plazaSvc *service.ModelPlazaService) *PlazaHandler {
	return &PlazaHandler{plazaSvc: plazaSvc}
}

func (h *PlazaHandler) GetPlaza(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	data, err := h.plazaSvc.BuildUserPlaza(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, (*dto.PlazaResponse)(data))
}
