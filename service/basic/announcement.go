package basic

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

type (
	ListAnnouncementService struct {
		Page     int `form:"page" binding:"min=1"`
		PageSize int `form:"page_size" binding:"min=1"`
	}
	ListAnnouncementParamCtx struct{}

	GetPopupService struct{}
)

// ListActive lists active announcements with pagination
func (s *ListAnnouncementService) ListActive(c *gin.Context) (*ListAnnouncementResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	pageSize := s.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	active := true
	res, err := announcementClient.List(c, &inventory.ListAnnouncementArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: pageSize,
			OrderBy:  "created_at",
			Order:    inventory.OrderDirectionDesc,
		},
		IsActive: &active,
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list announcements", err)
	}

	return &ListAnnouncementResponse{
		Pagination:    res.PaginationResults,
		Announcements: res.Announcements,
	}, nil
}

type ListAnnouncementResponse struct {
	Pagination    *inventory.PaginationResults `json:"pagination"`
	Announcements []*ent.Announcement          `json:"announcements"`
}

// GetPopup returns the popup announcement if popup is enabled.
func (s *GetPopupService) GetPopup(c *gin.Context) (*GetPopupResponse, error) {
	dep := dependency.FromContext(c)
	announcementClient := dep.AnnouncementClient()

	enabled := true
	if v, err := dep.SettingClient().Get(c, "announcement_popup_enabled"); err == nil {
		enabled = v == "1" || v == "true"
	}

	if !enabled {
		return &GetPopupResponse{Enabled: false}, nil
	}

	announcement, err := announcementClient.GetPopup(c)
	if err != nil {
		if ent.IsNotFound(err) {
			return &GetPopupResponse{Enabled: true}, nil
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get popup announcement", err)
	}

	return &GetPopupResponse{
		Enabled:      true,
		Announcement: announcement,
	}, nil
}

type GetPopupResponse struct {
	Enabled      bool                 `json:"enabled"`
	Announcement *ent.Announcement    `json:"announcement,omitempty"`
}
