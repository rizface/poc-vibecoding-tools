package service

type GetChatHistoryInput struct {
	OnlyLast  bool
	ProjectId string
}

type GetProjectFileInput struct {
	ProjectId string
}

type DeleteProjectInput struct {
	ProjectId string
}

type GetOneProjectInput struct {
	ProjectId string
}

type CreateOneProjectInput struct {
	Name string
}
