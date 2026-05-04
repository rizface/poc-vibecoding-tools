package repository

import (
	"time"

	"gorm.io/gorm"
)

type BasicModelColumn struct {
	ID        string    `gorm:"column:id;type:uuid"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type ContainerModel struct {
	BasicModelColumn
	ContainerId string       `gorm:"column:container_id"`
	HostPort    string       `gorm:"column:host_port"`
	Project     ProjectModel `gorm:"foreignKey:ContainerId;references:ID"`
}

func (cm ContainerModel) TableName() string {
	return "containers"
}

type ProjectModel struct {
	BasicModelColumn
	Name          string             `gorm:"column:name;not null"`
	Hostname      string             `gorm:"column:hostname;not null;unique"`
	ContainerId   string             `gorm:"column:container_id;type:uuid"`
	ProjectFiles  []ProjectFileModel `gorm:"foreignKey:ProjectId;references:ID;constraint:OnDelete:CASCADE;"`
	ChatHistories []ChatHistoryModel `gorm:"foreignKey:ProjectId;references:ID;constraint:OnDelete:CASCADE;"`
}

func (pm ProjectModel) TableName() string {
	return "projects"
}

type ProjectFileModel struct {
	BasicModelColumn
	ProjectID string `gorm:"column:project_id;type:uuid;uniqueIndex:idx_project_files_project_path"`
	Path      string `gorm:"column:path;not null;uniqueIndex:idx_project_files_project_path"`
}

func (pfm ProjectFileModel) TableName() string {
	return "project_files"
}

type ChatHistoryModel struct {
	BasicModelColumn
	ProjectID string `gorm:"column:project_id;type:uuid"`
	Chat      string `gorm:"column:chat;type:text;"`
	Response  string `gorm:"column:response;type:text;"`
}

func (chm ChatHistoryModel) TableName() string {
	return "chat_histories"
}

func AutoMigrate(db *gorm.DB) {
	db.AutoMigrate(
		&ContainerModel{},
		&ProjectModel{},
		&ProjectFileModel{},
		&ChatHistoryModel{},
	)
}
