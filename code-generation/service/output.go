package service

import (
	"fmt"
	"os"
	"time"

	"github.com/rizface/poc-code-generation/repository"
)

type ChatHistoryOutput struct {
	Id        string    `json:"id"`
	Chat      string    `json:"chat"`
	Response  string    `json:"response"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ChatHistoryutputFromModel(chat repository.ChatHistoryModel) ChatHistoryOutput {
	return ChatHistoryOutput{
		Id:        chat.ID,
		Chat:      chat.Chat,
		Response:  chat.Response,
		CreatedAt: chat.CreatedAt,
		UpdatedAt: chat.UpdatedAt,
	}
}

func ChatHistoriesOutputFromModels(chats []repository.ChatHistoryModel) []ChatHistoryOutput {
	outputs := []ChatHistoryOutput{}

	for _, chat := range chats {
		outputs = append(outputs, ChatHistoryutputFromModel(chat))
	}

	return outputs
}

type ProjectFileOutput struct {
	Id       string `json:"id"`
	Filename string `json:"filename"`
	Code     string `json:"code"`
}

func ProjectFilesOutputFromModels(files []repository.ProjectFileModel) ([]ProjectFileOutput, error) {
	outputs := []ProjectFileOutput{}

	for _, file := range files {
		filepath := fmt.Sprintf("%s/code-generation/%s/%s", "/home/fariz", file.ProjectID, file.Path)

		// improve later using bufio
		content, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		outputs = append(outputs, ProjectFileOutput{
			Id:       file.ID,
			Filename: file.Path,
			Code:     string(content),
		})
	}

	return outputs, nil
}

type ProjectOutput struct {
	Id        string    `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      string    `json:"port,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ProjectOutputFromModel(project repository.ProjectModel, container repository.ContainerModel) ProjectOutput {
	return ProjectOutput{
		Id:        project.ID,
		Name:      project.Name,
		Host:      project.Hostname,
		Port:      container.HostPort,
		CreatedAt: project.CreatedAt,
		UpdatedAt: project.UpdatedAt,
	}
}

func ProjectsOutputFromModels(projects []repository.ProjectModel) []ProjectOutput {
	outputs := []ProjectOutput{}

	for _, project := range projects {
		outputs = append(outputs, ProjectOutputFromModel(project, repository.ContainerModel{}))
	}

	return outputs
}

type CreateProjectOutput struct {
	ProjectId   string `json:"projectId"`
	Host        string `json:"host"`
	ContainerId string `json:"containerId"`
	HostPort    string `json:"hostPort"`
}
