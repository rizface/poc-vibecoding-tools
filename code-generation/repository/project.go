package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{
		db: db,
	}
}

func (pr *ProjectRepository) CreateOne(ctx context.Context, project ProjectRepository) error {
	return pr.db.WithContext(ctx).Create(&project).Error
}

func (pr *ProjectRepository) GetOneById(ctx context.Context, id string) (ProjectModel, error) {
	var project ProjectModel

	err := pr.db.WithContext(ctx).Where("id = ?", id).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, ErrNotFound
	}

	return project, err
}

func (pr *ProjectRepository) GetList(ctx context.Context) ([]ProjectModel, error) {
	projects := []ProjectModel{}

	return projects, pr.db.
		WithContext(ctx).
		Order("updated_at desc").
		Find(&projects).
		Error
}

func (pr *ProjectRepository) DeleteOne(ctx context.Context, id string) error {
	project, err := pr.GetOneById(ctx, id)
	if err != nil {
		return err
	}

	return pr.db.Delete(&project).Error
}
