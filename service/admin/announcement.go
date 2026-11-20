package admin

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

type (
	SingleAnnouncementService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	SingleAnnouncementParamCtx struct{}
)

type (
	UpsertAnnouncementRequest struct {
		ID       int    `json:"id,omitempty"`
		Title    string `json:"title" binding:"required"`
		Content  string `json:"content,omitempty"`
		IsActive bool   `json:"is_active"`
		IsPopup  bool   `json:"is_popup"`
	}
	UpsertAnnouncementService struct {
		Announcement *UpsertAnnouncementRequest `json:"announcement" binding:"required"`
	}
	UpsertAnnouncementParamCtx struct{}
)

type (
	BatchAnnouncementService struct {
		IDs []int `json:"ids" binding:"required"`
	}
	BatchAnnouncementParamCtx struct{}
)

// Announcements lists announcements with pagination
func (s *AdminListService) Announcements(c *gin.Context) (*ListAnnouncementResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	var isActive *bool
	if activeStr, ok := s.Conditions["is_active"]; ok {
		active := activeStr == "true"
		isActive = &active
	}

	res, err := announcementClient.List(c, &inventory.ListAnnouncementArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			OrderBy:  s.OrderBy,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
		IsActive: isActive,
		Keyword:   s.Searches["keyword"],
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list announcements", err)
	}

	return &ListAnnouncementResponse{
		Pagination:    res.PaginationResults,
		Announcements: res.Announcements,
	}, nil
}

func (s *SingleAnnouncementService) Get(c *gin.Context) (*GetAnnouncementResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	announcement, err := announcementClient.GetByID(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Announcement not found", err)
	}

	return &GetAnnouncementResponse{
		Announcement: announcement,
	}, nil
}

func (r *UpsertAnnouncementRequest) toEnt() *ent.Announcement {
	return &ent.Announcement{
		ID:       r.ID,
		Title:    r.Title,
		Content:  r.Content,
		IsActive: r.IsActive,
		IsPopup:  r.IsPopup,
	}
}

func (s *UpsertAnnouncementService) Create(c *gin.Context) (*GetAnnouncementResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	if s.Announcement.ID > 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID must be 0 for creating new announcement", nil)
	}

	announcement, err := announcementClient.Create(c, s.Announcement.toEnt())
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create announcement", err)
	}

	service := &SingleAnnouncementService{ID: announcement.ID}
	return service.Get(c)
}

func (s *UpsertAnnouncementService) Update(c *gin.Context) (*GetAnnouncementResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	if s.Announcement.ID == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID is required", nil)
	}

	// Check if announcement exists
	if _, err := announcementClient.GetByID(c, s.Announcement.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Announcement not found", err)
	}

	announcement, err := announcementClient.Update(c, s.Announcement.toEnt())
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update announcement", err)
	}

	service := &SingleAnnouncementService{ID: announcement.ID}
	return service.Get(c)
}

func (s *SingleAnnouncementService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	// Check if announcement exists
	if _, err := announcementClient.GetByID(c, s.ID); err != nil {
		return serializer.NewError(serializer.CodeNotFound, "Announcement not found", err)
	}

	if err := announcementClient.Delete(c, s.ID); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete announcement", err)
	}

	return nil
}

func (s *BatchAnnouncementService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	for _, id := range s.IDs {
		// Check if announcement exists
		if _, err := announcementClient.GetByID(c, id); err != nil {
			continue // Skip non-existent announcements
		}

		announcementClient.Delete(c, id)
	}

	return nil
}
