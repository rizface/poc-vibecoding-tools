package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rizface/poc-code-generation/service"
)

type ProjectHandler struct {
	projectService *service.ProjectService
}

func NewProjectHandler(
	ProjectService *service.ProjectService,
) *ProjectHandler {
	return &ProjectHandler{
		projectService: ProjectService,
	}
}

func (pfh *ProjectHandler) CreateProject(c *gin.Context) {
	var payload struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"err": err.Error()})
		return
	}

	project, err := pfh.projectService.CreateProject(c.Request.Context(), service.CreateOneProjectInput{
		Name: payload.Name,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"doc": project,
		},
	})
}

func (pfh *ProjectHandler) GetListProject(c *gin.Context) {
	projects, err := pfh.projectService.GetListProject(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"docs": projects,
		},
	})
}

func (pfh *ProjectHandler) GetOneProject(c *gin.Context) {
	projectId := c.Param("id")

	project, err := pfh.projectService.GetOneProject(c.Request.Context(), service.GetOneProjectInput{
		ProjectId: projectId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"doc": project,
		},
	})
}

func (pfh *ProjectHandler) DeleteProject(c *gin.Context) {
	projectId := c.Param("id")

	err := pfh.projectService.DeleteProject(c.Request.Context(), service.DeleteProjectInput{
		ProjectId: projectId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"error":  nil,
		"status": "deleted",
	})
}

func (pfh *ProjectHandler) GetProjectFiles(c *gin.Context) {
	projectId := c.Param("id")

	files, err := pfh.projectService.GetProjectFileList(c.Request.Context(), service.GetProjectFileInput{
		ProjectId: projectId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"docs": files,
		},
	})
}
