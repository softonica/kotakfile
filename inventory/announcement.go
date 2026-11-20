package inventory

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/announcement"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
)

type (
	AnnouncementClient interface {
		TxOperator
		// List returns a paginated list of announcements.
		List(ctx context.Context, args *ListAnnouncementArgs) (*ListAnnouncementResult, error)
		// GetByID returns the announcement by its ID.
		GetByID(ctx context.Context, id int) (*ent.Announcement, error)
		// GetPopup returns the latest active announcement that is marked as popup.
		GetPopup(ctx context.Context) (*ent.Announcement, error)
		// Create creates a new announcement.
		Create(ctx context.Context, a *ent.Announcement) (*ent.Announcement, error)
		// Update updates an existing announcement.
		Update(ctx context.Context, a *ent.Announcement) (*ent.Announcement, error)
		// Delete deletes an announcement by its ID.
		Delete(ctx context.Context, id int) error
	}

	ListAnnouncementArgs struct {
		*PaginationArgs
		IsActive *bool
		Keyword  string
	}

	ListAnnouncementResult struct {
		*PaginationResults
		Announcements []*ent.Announcement
	}
)

func NewAnnouncementClient(client *ent.Client, dbType conf.DBType) AnnouncementClient {
	return &announcementClient{
		client:      client,
		maxSQlParam: sqlParamLimit(dbType),
	}
}

type announcementClient struct {
	client      *ent.Client
	maxSQlParam int
}

func (c *announcementClient) SetClient(newClient *ent.Client) TxOperator {
	return &announcementClient{client: newClient, maxSQlParam: c.maxSQlParam}
}

func (c *announcementClient) GetClient() *ent.Client {
	return c.client
}

func (c *announcementClient) List(ctx context.Context, args *ListAnnouncementArgs) (*ListAnnouncementResult, error) {
	query := c.client.Announcement.Query()

	if args.IsActive != nil {
		query.Where(announcement.IsActive(*args.IsActive))
	}

	if args.Keyword != "" {
		query.Where(
			announcement.Or(
				announcement.TitleContains(args.Keyword),
				announcement.ContentContains(args.Keyword),
			),
		)
	}

	pageSize := capPageSize(c.maxSQlParam, args.PageSize, 1)

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count announcements: %w", err)
	}

	query.Order(getAnnouncementOrderOption(args)...)

	announcements, err := query.
		Limit(pageSize).
		Offset(args.Page * pageSize).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list announcements: %w", err)
	}

	return &ListAnnouncementResult{
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
		Announcements: announcements,
	}, nil
}

func (c *announcementClient) GetByID(ctx context.Context, id int) (*ent.Announcement, error) {
	return c.client.Announcement.Query().
		Where(announcement.ID(id)).
		First(ctx)
}

func (c *announcementClient) GetPopup(ctx context.Context) (*ent.Announcement, error) {
	return c.client.Announcement.Query().
		Where(announcement.IsActive(true), announcement.IsPopup(true)).
		Order(announcement.ByCreatedAt(sql.OrderDesc()), announcement.ByID(sql.OrderDesc())).
		First(ctx)
}

func (c *announcementClient) Create(ctx context.Context, a *ent.Announcement) (*ent.Announcement, error) {
	return c.client.Announcement.Create().
		SetTitle(a.Title).
		SetContent(a.Content).
		SetIsActive(a.IsActive).
		SetIsPopup(a.IsPopup).
		Save(ctx)
}

func (c *announcementClient) Update(ctx context.Context, a *ent.Announcement) (*ent.Announcement, error) {
	return c.client.Announcement.UpdateOneID(a.ID).
		SetTitle(a.Title).
		SetContent(a.Content).
		SetIsActive(a.IsActive).
		SetIsPopup(a.IsPopup).
		Save(ctx)
}

func (c *announcementClient) Delete(ctx context.Context, id int) error {
	return c.client.Announcement.DeleteOneID(id).Exec(ctx)
}

func getAnnouncementOrderOption(args *ListAnnouncementArgs) []announcement.OrderOption {
	orderTerm := getOrderTerm(args.Order)
	switch args.OrderBy {
	case announcement.FieldTitle:
		return []announcement.OrderOption{announcement.ByTitle(orderTerm), announcement.ByID(orderTerm)}
	case announcement.FieldCreatedAt:
		return []announcement.OrderOption{announcement.ByCreatedAt(orderTerm), announcement.ByID(orderTerm)}
	case announcement.FieldUpdatedAt:
		return []announcement.OrderOption{announcement.ByUpdatedAt(orderTerm), announcement.ByID(orderTerm)}
	case announcement.FieldIsActive:
		return []announcement.OrderOption{announcement.ByIsActive(orderTerm), announcement.ByID(orderTerm)}
	default:
		return []announcement.OrderOption{announcement.ByID(orderTerm)}
	}
}
