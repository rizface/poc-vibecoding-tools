package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	dockerClient "github.com/moby/moby/client"
	"gorm.io/gorm/clause"
)

func ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

func generateStream(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"err": "streaming unsupported"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	sendEvent := func(event string, data interface{}) {
		c.SSEvent(event, data)
		flusher.Flush()
	}

	chatHistories := []ChatHistoryModel{}
	err := gormDB.WithContext(c.Request.Context()).
		Where("project_id = ?", payload.ProjectId).
		Order("created_at asc").
		Find(&chatHistories).
		Error
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	chatHistories = append(chatHistories, ChatHistoryModel{
		Chat: payload.Prompt,
	})

	projectFiles := []ProjectFileModel{}
	err = gormDB.WithContext(c.Request.Context()).
		Where("project_id = ?", payload.ProjectId).
		Find(&projectFiles).
		Error
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	var geminiFullResponseChat strings.Builder
	clientIsGone := c.Request.Context().Done()

	chatStream, err := streamChat(c.Request.Context(), payload.ProjectId, chatHistories, projectFiles)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientIsGone:
			return false
		case msg, ok := <-chatStream:
			if ok {
				c.SSEvent("message", msg)
				geminiFullResponseChat.WriteString(msg)
			}

			return ok
		}
	})

	var geminiParsedResponseChat GenAIResponseChatStream

	if geminiFullResponseChat.String() == "" {
		return
	}

	err = json.Unmarshal([]byte(geminiFullResponseChat.String()), &geminiParsedResponseChat)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	err = gormDB.WithContext(c.Request.Context()).Create(&ChatHistoryModel{
		BasicModelColumn: BasicModelColumn{
			ID:        uuid.NewString(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		ProjectID: payload.ProjectId,
		Chat:      payload.Prompt,
		Response:  geminiParsedResponseChat.Response,
	}).Error
	if err != nil {
		fmt.Printf("[ERROR]: failed insert last chat to database: %v", err.Error())
	}

	sendEvent("done", geminiParsedResponseChat)

	if !geminiParsedResponseChat.ReadyToExecute {
		return
	}

	fmt.Println("response after ready", geminiParsedResponseChat)

	reqStream, err := streamRequirement(c.Request.Context(), chatHistories, projectFiles)
	if err != nil {
		fmt.Printf("[ERROR]: failed generate requirement: %v", err.Error())
		sendEvent("error", err.Error())
		return
	}

	var geminiFullRequirementResponse strings.Builder

	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientIsGone:
			return false
		case msg, ok := <-reqStream:
			if ok {
				c.SSEvent("requirement", msg)
				geminiFullRequirementResponse.WriteString(msg)
			}
			return ok
		}
	})

	var spec GenAIResponseRequirement
	if geminiFullRequirementResponse.String() == "" {
		return
	}

	err = json.Unmarshal([]byte(geminiFullRequirementResponse.String()), &spec)
	if err != nil {
		fmt.Printf("[ERROR]: failed parse requirement: %v", err.Error())
		sendEvent("error", err.Error())
		return
	}

	fmt.Println(spec)

	var geminiFullResponse strings.Builder

	codeStream, err := streamCodeGeneration(c.Request.Context(), spec, projectFiles)
	if err != nil {
		fmt.Printf("[ERROR]: failed generate project: %v", err.Error())
		sendEvent("error", err.Error())
		return
	}

	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientIsGone:
			return false
		case msg, ok := <-codeStream:
			if ok {
				c.SSEvent("message", msg)
				geminiFullResponse.WriteString(msg)
			}

			return ok
		}
	})

	var geminiParsedResponse struct {
		Contents []GenAIResponse `json:"contents"`
	}

	if geminiFullResponse.String() == "" {
		return
	}

	err = json.Unmarshal([]byte(geminiFullResponse.String()), &geminiParsedResponse)
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	projectFiles = []ProjectFileModel{}

	for _, content := range geminiParsedResponse.Contents {
		path := fmt.Sprintf("%s/code-generation/%s/%s", homeDir, payload.ProjectId, content.Filename)
		pathPart := strings.Split(path, "/")
		dir := strings.Join(pathPart[:len(pathPart)-1], "/")

		err := os.MkdirAll(dir, 0755)
		if err != nil {
			sendEvent("error", err.Error())
			return
		}

		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			sendEvent("error", err.Error())
			return
		}
		defer file.Close()

		writer := bufio.NewWriter(file)

		_, err = writer.WriteString(content.Code)
		if err != nil {
			sendEvent("error", err.Error())
			return
		}

		err = writer.Flush()
		if err != nil {
			sendEvent("error", err.Error())
			return
		}

		projectFiles = append(projectFiles, ProjectFileModel{
			BasicModelColumn: BasicModelColumn{
				ID:        uuid.NewString(),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			ProjectID: payload.ProjectId,
			Path:      content.Filename,
		})
	}

	err = gormDB.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "path"}},
			DoNothing: true,
		}).
		Create(&projectFiles).Error
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	type donePayload struct {
		Files []GenAIResponse `json:"files"`
	}

	doneBytes, err := json.Marshal(donePayload{Files: geminiParsedResponse.Contents})
	if err != nil {
		sendEvent("error", err.Error())
		return
	}

	sendEvent("done", string(doneBytes))
}

func getOneProject(c *gin.Context) {
	id := c.Param("id")

	var project ProjectModel
	if err := gormDB.First(&project, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"err": "project not found"})
		return
	}

	var container ContainerModel
	gormDB.First(&container, "id = ?", project.ContainerId)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"projectId": project.ID,
			"name":      project.Name,
			"host":      project.Hostname,
			"hostPort":  container.HostPort,
		},
	})
}

func getProjectFiles(c *gin.Context) {
	id := c.Param("id")
	files := []ProjectFileModel{}

	err := gormDB.Find(&files, "project_id = ?", id).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	filesAndCode := []GenAIResponse{}

	for _, entry := range files {
		filepath := fmt.Sprintf("%s/code-generation/%s/%s", homeDir, id, entry.Path)

		content, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		filesAndCode = append(filesAndCode, GenAIResponse{
			Filename: entry.Path,
			Code:     string(content),
		})
	}

	c.JSON(http.StatusOK, gin.H{"files": filesAndCode})
}

func listProjects(c *gin.Context) {
	var projects []ProjectModel

	err := gormDB.Order("created_at desc").Find(&projects).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	type projectItem struct {
		ProjectId string    `json:"projectId"`
		Name      string    `json:"name"`
		Host      string    `json:"host"`
		UpdatedAt time.Time `json:"updatedAt"`
	}

	result := make([]projectItem, len(projects))
	for i, p := range projects {
		result[i] = projectItem{ProjectId: p.ID, Name: p.Name, Host: p.Hostname, UpdatedAt: p.UpdatedAt}
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func deleteProject(c *gin.Context) {
	id := c.Param("id")

	var project ProjectModel
	if err := gormDB.First(&project, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"err": "project not found"})
		return
	}

	var container ContainerModel
	gormDB.First(&container, "id = ?", project.ContainerId)

	if container.ContainerId != "" {
		_, err := dockerApiClient.ContainerRemove(c.Request.Context(), container.ContainerId, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
		if err != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", container.ContainerId, err.Error())
		}
	}

	if err := gormDB.Delete(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	projectPath := fmt.Sprintf("%s/code-generation/%s", homeDir, id)
	if err := os.RemoveAll(projectPath); err != nil {
		fmt.Printf("[ERROR]: failed remove project path %s; %s \n", projectPath, err.Error())
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func getChatHistory(c *gin.Context) {
	id := c.Param("id")
	onlyLast := c.Query("only_last_chat") == "true"

	var histories []ChatHistoryModel
	q := gormDB.Where("project_id = ?", id).Order("created_at desc")
	if onlyLast {
		q = q.Limit(1)
	}
	if err := q.Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	if !onlyLast {
		for i, j := 0, len(histories)-1; i < j; i, j = i+1, j-1 {
			histories[i], histories[j] = histories[j], histories[i]
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": histories})
}

func createOneProject(c *gin.Context) {
	var payload struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"err": err.Error()})
		return
	}

	project := ProjectModel{
		BasicModelColumn: BasicModelColumn{
			ID:        uuid.NewString(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Name: payload.Name,
	}

	// create project dir
	projectPath := fmt.Sprintf("%s/code-generation/%s", homeDir, project.ID)

	err := os.Mkdir(projectPath, 0777)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	// create container
	createContainerCfg := createContainerConfig{
		name: project.ID,
		binds: []string{
			fmt.Sprintf("%s:/usr/share/nginx/html", projectPath),
		},
	}

	createdContainer, err := createContainer(c.Request.Context(), createContainerCfg)
	if err != nil {
		errRemoveProjectPath := os.Remove(projectPath)
		if errRemoveProjectPath != nil {
			fmt.Printf("[ERROR]: failed remove project path %s; %s \n", projectPath, errRemoveProjectPath.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	project.Hostname = createdContainer.host

	container := ContainerModel{
		BasicModelColumn: BasicModelColumn{
			ID:        uuid.NewString(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		ContainerId: createdContainer.containerId,
		HostPort:    createdContainer.containerPort,
		Project:     project,
	}

	err = gormDB.Create(&container).Error
	if err != nil {
		errRemoveProjectPath := os.Remove(projectPath)
		if errRemoveProjectPath != nil {
			fmt.Printf("[ERROR]: failed remove project path %s; %s \n", projectPath, errRemoveProjectPath.Error())
		}

		_, errRemoveContainer := dockerApiClient.ContainerRemove(c.Request.Context(), createdContainer.containerId, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})
		if errRemoveContainer != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", createdContainer.containerId, errRemoveContainer.Error())
		}

		c.JSON(http.StatusInternalServerError, gin.H{"err": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success create container",
		"data": gin.H{
			"projectId":   project.ID,
			"host":        project.Hostname,
			"containerId": createdContainer.containerId,
			"hostPort":    createdContainer.containerPort,
		},
	})
}
