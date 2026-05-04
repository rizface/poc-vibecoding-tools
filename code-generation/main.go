package main

import (
	"log"
	"os"

	"context"
	"github.com/gin-gonic/gin"
	dockerClient "github.com/moby/moby/client"
	"github.com/rizface/poc-code-generation/container"
	"github.com/rizface/poc-code-generation/handler"
	"github.com/rizface/poc-code-generation/repository"
	"github.com/rizface/poc-code-generation/service"
)

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	ctx := context.Background()
	config := loadConfig()

	geminiClient, err := openGeminiClient(ctx, config)
	panicIfErr(err)

	homeDir, err := os.UserHomeDir()
	panicIfErr(err)

	db, err := openDB(config)
	panicIfErr(err)

	repository.AutoMigrate(db)

	rawDockerClient, err := dockerClient.New(dockerClient.FromEnv)
	panicIfErr(err)

	dockerCli := container.NewDockerClient(rawDockerClient)

	projectRepo := repository.NewProjectRepository(db)
	containerRepo := repository.NewContainerRepository(db)
	projectFileRepo := repository.NewProjectFileRepository(db)
	chatHistoryRepo := repository.NewChatHistoryRepository(db)

	projectSvc := service.NewProjectFileService(homeDir, projectRepo, containerRepo, projectFileRepo, dockerCli)
	chatHistorySvc := service.NewChatHistoryService(chatHistoryRepo)
	generationSvc := service.NewGenerationService(homeDir, geminiClient, chatHistoryRepo, projectFileRepo)

	projectHandler := handler.NewProjectHandler(projectSvc)
	chatHistoryHandler := handler.NewChatHistoryHandler(chatHistorySvc)
	generationHandler := handler.NewGenerationHandler(generationSvc)

	r := gin.Default()

	r.GET("/ping", ping)
	r.POST("/action/generate-stream", generationHandler.GenerateStream)
	r.GET("/project", projectHandler.GetListProject)
	r.POST("/project", projectHandler.CreateProject)
	r.GET("/project/:id", projectHandler.GetOneProject)
	r.DELETE("/project/:id", projectHandler.DeleteProject)
	r.GET("/project/:id/files", projectHandler.GetProjectFiles)
	r.GET("/project/:id/chat-history", chatHistoryHandler.GetChatHistory)

	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
