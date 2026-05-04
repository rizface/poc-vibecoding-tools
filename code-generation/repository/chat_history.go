package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type ChatHistoryRepository struct {
	db *gorm.DB
}

func NewChatHistoryRepository(db *gorm.DB) *ChatHistoryRepository {
	return &ChatHistoryRepository{
		db: db,
	}
}

func (chr *ChatHistoryRepository) GetListByProject(ctx context.Context, projectId string) ([]ChatHistoryModel, error) {
	chats := []ChatHistoryModel{}

	return chats, chr.db.WithContext(ctx).Where("project_id = ?", projectId).Order("created_at asc").Find(&chats).Error
}

func (chr *ChatHistoryRepository) GetLastChat(ctx context.Context, projectId string) (ChatHistoryModel, error) {
	var chat ChatHistoryModel

	err := chr.db.
		WithContext(ctx).
		Where("project_id = ?", projectId).
		Order("created_at desc").
		First(&chat).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return chat, nil
	}

	return chat, err
}

func (chr *ChatHistoryRepository) CreateOne(ctx context.Context, chat ChatHistoryModel) error {
	return chr.db.WithContext(ctx).
		Create(&chat).Error
}
