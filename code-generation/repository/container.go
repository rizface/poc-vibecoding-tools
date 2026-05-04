package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type ContainerRepository struct {
	db *gorm.DB
}

func NewContainerRepository(db *gorm.DB) *ContainerRepository {
	return &ContainerRepository{
		db: db,
	}
}

func (cr *ContainerRepository) GetOneById(ctx context.Context, id string) (ContainerModel, error) {
	var container ContainerModel

	err := cr.db.WithContext(ctx).First(&container, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return container, ErrNotFound
	}

	if err != nil {
		return container, err
	}

	return container, nil
}

func (cr *ContainerRepository) Create(ctx context.Context, container ContainerModel) error {
	return cr.db.WithContext(ctx).Create(&container).Error
}

func (cr *ContainerRepository) Delete(ctx context.Context, id string) error {
	return cr.db.WithContext(ctx).Where("id = ?", id).Delete(&ContainerModel{}).Error
}
