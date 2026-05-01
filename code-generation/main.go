package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	dockerClient "github.com/moby/moby/client"
	"google.golang.org/genai"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var homeDir string

func openDB() (*gorm.DB, error) {
	dsn := "host=localhost user=postgres password=postgres dbname=postgres port=5432 sslmode=disable TimeZone=Asia/Jakarta"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	return db, err
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func openGeminiClient(ctx context.Context) error {
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return err
	}

	client = genaiClient

	return nil
}

func main() {
	ctx := context.Background()

	err := openGeminiClient(ctx)
	panicIfErr(err)

	homeDir, err = os.UserHomeDir()
	panicIfErr(err)

	db, err := openDB()
	panicIfErr(err)

	gormDB = db

	db.AutoMigrate(
		&ContainerModel{},
		&ProjectModel{},
		&ProjectFileModel{},
		&ChatHistoryModel{},
	)

	dockerApiClient, err = dockerClient.New(dockerClient.FromEnv)
	panicIfErr(err)

	r := gin.Default()

	r.GET("/ping", ping)
	r.POST("/action/generate-stream", generateStream)
	r.GET("/project", listProjects)
	r.POST("/project", createOneProject)
	r.GET("/project/:id", getOneProject)
	r.DELETE("/project/:id", deleteProject)
	r.GET("/project/:id/files", getProjectFiles)
	r.GET("/project/:id/chat-history", getChatHistory)

	r.POST("/start-container", func(c *gin.Context) {
	})

	r.POST("/stop-container", func(c *gin.Context) {})

	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
