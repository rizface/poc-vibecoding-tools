package service

import (
	"context"

	"github.com/rizface/poc-code-generation/repository"
)

type ChatHistoryService struct {
	chatHistoryRepo *repository.ChatHistoryRepository
}

func NewChatHistoryService(
	chatHistoryRepo *repository.ChatHistoryRepository,
) *ChatHistoryService {
	return &ChatHistoryService{
		chatHistoryRepo: chatHistoryRepo,
	}
}

func (chs *ChatHistoryService) GetChatHistory(ctx context.Context, input GetChatHistoryInput) ([]ChatHistoryOutput, error) {
	if input.OnlyLast {
		chat, err := chs.chatHistoryRepo.GetLastChat(ctx, input.ProjectId)
		if err != nil {
			return []ChatHistoryOutput{}, err
		}

		return ChatHistoriesOutputFromModels([]repository.ChatHistoryModel{chat}), nil
	}

	chats, err := chs.chatHistoryRepo.GetListByProject(ctx, input.ProjectId)
	if err != nil {
		return []ChatHistoryOutput{}, err
	}

	return ChatHistoriesOutputFromModels(chats), nil
}
