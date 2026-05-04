package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rizface/poc-code-generation/service"
)

type GenerationHandler struct {
	generationService *service.GenerationService
}

type SSEStandardOutput struct {
	MessageComplete bool `json:"messageComplete"`
	Data            any  `json:"data"`
}

func NewGenerationHandler(generationService *service.GenerationService) *GenerationHandler {
	return &GenerationHandler{generationService: generationService}
}

func (gh *GenerationHandler) GenerateStream(c *gin.Context) {
	var payload struct {
		ProjectId string `json:"projectId"`
		Prompt    string `json:"prompt"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	sendEvent := func(event string, data any) {
		c.SSEvent(event, data)
		flusher.Flush()
	}

	clientGone := c.Request.Context().Done()

	chatHistories, projectFiles, err := gh.generationService.GetChatHistoriesForStream(
		c.Request.Context(), payload.ProjectId, payload.Prompt,
	)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	chatStream, err := gh.generationService.StreamChat(c.Request.Context(), chatHistories, projectFiles)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	var fullChatResponse strings.Builder
	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientGone:
			return false
		case msg, ok := <-chatStream:
			if ok {
				fullChatResponse.WriteString(msg)
				c.SSEvent("chatGeneration", SSEStandardOutput{
					MessageComplete: false,
					Data:            msg,
				})
			}
			return ok
		}
	})

	if fullChatResponse.Len() == 0 {
		return
	}

	var chatResponse service.GenAIChatResponse
	if err := json.Unmarshal([]byte(fullChatResponse.String()), &chatResponse); err != nil {
		sendEvent("error", err.Error())
		return
	}

	if err := gh.generationService.SaveChatHistory(
		c.Request.Context(), payload.ProjectId, payload.Prompt, chatResponse.Response,
	); err != nil {
		fmt.Printf("[ERROR]: failed insert chat history to database: %v\n", err.Error())
	}

	sendEvent("doneChatGeneration", SSEStandardOutput{
		MessageComplete: true,
		Data:            chatResponse,
	})

	if !chatResponse.ReadyToExecute {
		return
	}

	reqStream, err := gh.generationService.StreamRequirement(c.Request.Context(), chatHistories, projectFiles)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	var fullRequirementResponse strings.Builder
	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientGone:
			return false
		case msg, ok := <-reqStream:
			if ok {
				fullRequirementResponse.WriteString(msg)

				c.SSEvent("requirementGeneration", SSEStandardOutput{
					MessageComplete: false,
					Data:            msg,
				})
			}

			return ok
		}
	})

	if fullRequirementResponse.Len() == 0 {
		return
	}

	var spec service.GenAIRequirementResponse
	if err := json.Unmarshal([]byte(fullRequirementResponse.String()), &spec); err != nil {
		sendEvent("error", err.Error())
		return
	}

	codeStream, err := gh.generationService.StreamCode(c.Request.Context(), spec, projectFiles)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	var fullCodeResponse strings.Builder
	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientGone:
			return false
		case msg, ok := <-codeStream:
			if ok {
				fullCodeResponse.WriteString(msg)

				c.SSEvent("codeGeneration", SSEStandardOutput{
					MessageComplete: false,
					Data:            msg,
				})
			}

			return ok
		}
	})

	if fullCodeResponse.Len() == 0 {
		return
	}

	var codeResponse struct {
		Contents []service.GenAIFile `json:"contents"`
	}
	if err := json.Unmarshal([]byte(fullCodeResponse.String()), &codeResponse); err != nil {
		sendEvent("error", err.Error())
		return
	}

	if err := gh.generationService.SaveGeneratedFiles(
		c.Request.Context(), payload.ProjectId, codeResponse.Contents,
	); err != nil {
		sendEvent("error", err.Error())
		return
	}

	type donePayload struct {
		Files []service.GenAIFile `json:"files"`
	}
	doneBytes, err := json.Marshal(donePayload{Files: codeResponse.Contents})
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	sendEvent("doneCodeGeneration", SSEStandardOutput{
		MessageComplete: true,
		Data:            string(doneBytes),
	})
}
