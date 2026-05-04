package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rizface/poc-code-generation/service"
)

type ChatHistoryHandler struct {
	chatHistoryService *service.ChatHistoryService
}

func NewChatHistoryHandler(
	chatHistoryService *service.ChatHistoryService,
) *ChatHistoryHandler {
	return &ChatHistoryHandler{
		chatHistoryService: chatHistoryService,
	}
}

func (chh *ChatHistoryHandler) GetChatHistory(c *gin.Context) {
	projectID := c.Param("id")
	onlyLast := c.Query("only_last_chat") == "true"

	history, err := chh.chatHistoryService.GetChatHistory(c.Request.Context(), service.GetChatHistoryInput{
		OnlyLast:  onlyLast,
		ProjectId: projectID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"docs": history,
		},
	})
}
