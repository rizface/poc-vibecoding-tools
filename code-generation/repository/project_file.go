package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectFileRepository struct {
	db *gorm.DB
}

func NewProjectFileRepository(db *gorm.DB) *ProjectFileRepository {
	return &ProjectFileRepository{
		db: db,
	}
}

func (pfr *ProjectFileRepository) GetListByProjectId(ctx context.Context, projectId string) ([]ProjectFileModel, error) {
	projectFiles := []ProjectFileModel{}

	return projectFiles, pfr.db.WithContext(ctx).Where("project_id = ?", projectId).Find(&projectFiles).Error
}

func (pfr *ProjectFileRepository) CreateBatch(ctx context.Context, files []ProjectFileModel) error {
	return pfr.db.
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "path"}},
			DoNothing: true,
		}).
		Create(&files).
		Error
}
