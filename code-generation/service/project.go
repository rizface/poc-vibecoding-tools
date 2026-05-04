package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rizface/poc-code-generation/container"
	"github.com/rizface/poc-code-generation/repository"
)

type ProjectService struct {
	homeDir         string
	projectRepo     *repository.ProjectRepository
	containerRepo   *repository.ContainerRepository
	projectFileRepo *repository.ProjectFileRepository
	dockerClient    *container.DockerClient
}

func NewProjectFileService(
	homeDir string,
	projectRepo *repository.ProjectRepository,
	containerRepo *repository.ContainerRepository,
	projectFileRepo *repository.ProjectFileRepository,
	dockerClient *container.DockerClient,
) *ProjectService {
	return &ProjectService{
		homeDir:         homeDir,
		projectRepo:     projectRepo,
		containerRepo:   containerRepo,
		projectFileRepo: projectFileRepo,
		dockerClient:    dockerClient,
	}
}

func createProjectDir(homeDir, id string) (string, error) {
	projectPath := fmt.Sprintf("%s/code-generation/%s", homeDir, id)

	return projectPath, os.Mkdir(projectPath, 0777)
}

func (ps *ProjectService) CreateProject(ctx context.Context, input CreateOneProjectInput) (CreateProjectOutput, error) {
	now := time.Now()
	projectId := uuid.NewString()

	projectPath, err := createProjectDir(ps.homeDir, projectId)
	if err != nil {
		return CreateProjectOutput{}, err
	}

	nginxContainer, err := ps.dockerClient.CreateNginxSandboxContainer(ctx, container.CreateNginxSandboxContainerInput{
		Name: input.Name,
		Binds: []string{
			fmt.Sprintf("%s:/usr/share/nginx/html", projectPath),
		},
	})
	if err != nil {
		if err := os.Remove(projectPath); err != nil {
			fmt.Printf("[ERROR]: failed remove project path %s; %s \n", projectPath, err.Error())

		}
		return CreateProjectOutput{}, err
	}

	project := repository.ProjectModel{
		BasicModelColumn: repository.BasicModelColumn{
			ID:        projectId,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     input.Name,
		Hostname: nginxContainer.Host,
	}
	newContainer := repository.ContainerModel{
		BasicModelColumn: repository.BasicModelColumn{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		ContainerId: nginxContainer.ID,
		HostPort:    nginxContainer.Port,
		Project:     project,
	}

	if err := ps.containerRepo.Create(ctx, newContainer); err != nil {
		var errs error

		if err := os.Remove(projectPath); err != nil {
			fmt.Printf("[ERROR]: failed remove project path %s; %s \n", projectPath, err.Error())
			errs = errors.Join(errs, err)
		}

		if err := ps.dockerClient.RemoveContainer(ctx, container.RemoveContainerInput{
			ID: newContainer.ContainerId,
		}); err != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", newContainer.ContainerId, err.Error())
			errs = errors.Join(errs, err)
		}

		return CreateProjectOutput{}, errs
	}

	return CreateProjectOutput{
		ProjectId:   project.ID,
		Host:        project.Hostname,
		ContainerId: newContainer.ID,
		HostPort:    newContainer.HostPort,
	}, nil
}

func (ps *ProjectService) GetListProject(ctx context.Context) ([]ProjectOutput, error) {
	projects, err := ps.projectRepo.GetList(ctx)
	if err != nil {
		return []ProjectOutput{}, err
	}

	return ProjectsOutputFromModels(projects), nil
}

func (ps *ProjectService) GetOneProject(ctx context.Context, input GetOneProjectInput) (ProjectOutput, error) {
	project, err := ps.projectRepo.GetOneById(ctx, input.ProjectId)
	if err != nil {
		return ProjectOutput{}, err
	}

	container, err := ps.containerRepo.GetOneById(ctx, project.ContainerId)
	if err != nil {
		return ProjectOutput{}, err
	}

	return ProjectOutputFromModel(project, container), nil
}

func (ps *ProjectService) DeleteProject(ctx context.Context, input DeleteProjectInput) error {
	project, err := ps.projectRepo.GetOneById(ctx, input.ProjectId)
	if err != nil {
		return err
	}

	projectContainer, err := ps.containerRepo.GetOneById(ctx, project.ContainerId)
	if err != nil {
		return err
	}

	err = ps.dockerClient.RemoveContainer(ctx, container.RemoveContainerInput{
		ID: projectContainer.ContainerId,
	})
	if err != nil {
		return err
	}

	err = ps.projectRepo.DeleteOne(ctx, project.ID)
	if err != nil {
		return err
	}

	err = ps.containerRepo.Delete(ctx, project.ContainerId)
	if err != nil {
		return err
	}

	return nil
}

func (ps *ProjectService) GetProjectFileList(ctx context.Context, input GetProjectFileInput) ([]ProjectFileOutput, error) {
	files, err := ps.projectFileRepo.GetListByProjectId(ctx, input.ProjectId)
	if err != nil {
		return []ProjectFileOutput{}, err
	}

	filesWithCode, err := ProjectFilesOutputFromModels(files)
	if err != nil {
		return []ProjectFileOutput{}, err
	}

	return filesWithCode, nil
}
