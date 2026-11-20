package admin

import (
	"context"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// AddGroupService 用户组添加服务
type AddGroupService struct {
	//Group model.Group `json:"group" binding:"required"`
}

// GroupService 用户组ID服务
type GroupService struct {
	ID uint `uri:"id" json:"id" binding:"required"`
}

// Get 获取用户组详情
func (service *GroupService) Get() serializer.Response {
	//group, err := model.GetGroupByID(service.ID)
	//if err != nil {
	//	return serializer.ErrDeprecated(serializer.CodeGroupNotFound, "", err)
	//}
	//
	//return serializer.Response{Data: group}

	return serializer.Response{}
}

// Delete 删除用户组
func (service *GroupService) Delete() serializer.Response {
	//// 查找用户组
	//group, err := model.GetGroupByID(service.ID)
	//if err != nil {
	//	return serializer.ErrDeprecated(serializer.CodeGroupNotFound, "", err)
	//}
	//
	//// 是否为系统用户组
	//if group.ID <= 3 {
	//	return serializer.ErrDeprecated(serializer.CodeInvalidActionOnSystemGroup, "", err)
	//}
	//
	//// 检查是否有用户使用
	//total := 0
	//row := model.DB.Model(&model.User{}).Where("group_id = ?", service.ID).
	//	Select("count(id)").Row()
	//row.Scan(&total)
	//if total > 0 {
	//	return serializer.ErrDeprecated(serializer.CodeGroupUsedByUser, strconv.Itoa(total), nil)
	//}
	//
	//model.DB.Delete(&group)

	return serializer.Response{}
}

func (service *SingleGroupService) Delete(c *gin.Context) error {
	if service.ID <= 3 {
		return serializer.NewError(serializer.CodeInvalidActionOnSystemGroup, "", nil)
	}

	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	// Any user still under this group?
	users, err := groupClient.CountUsers(c, int(service.ID))
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to count users", err)
	}

	if users > 0 {
		return serializer.NewError(serializer.CodeGroupUsedByUser, strconv.Itoa(users), nil)
	}

	err = groupClient.Delete(c, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete group", err)
	}

	return nil
}

func (s *AdminListService) List(c *gin.Context) (*ListGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	ctx := context.WithValue(c, inventory.LoadGroupPolicy{}, true)
	res, err := groupClient.ListGroups(ctx, &inventory.ListGroupParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			OrderBy:  s.OrderBy,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list groups", err)
	}

	return &ListGroupResponse{
		Pagination: res.PaginationResults,
		Groups:     res.Groups,
	}, nil
}

type (
	SingleGroupService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	SingleGroupParamCtx struct{}
)

const (
	countUserQuery = "countUser"
)

func (s *SingleGroupService) Get(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	ctx := context.WithValue(c, inventory.LoadGroupPolicy{}, true)
	group, err := groupClient.GetByID(ctx, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get group", err)
	}

	res := &GetGroupResponse{Group: group}

	if c.Query(countUserQuery) != "" {
		totalUsers, err := groupClient.CountUsers(ctx, int(s.ID))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to count users", err)
		}
		res.TotalUsers = totalUsers
	}

	return res, nil
}

type (
	UpsertGroupService struct {
		Group *ent.Group `json:"group" binding:"required"`
	}
	UpsertGroupParamCtx struct{}
)

// validateAssignedPolicies makes sure the group's assigned policy set is
// coherent: an empty set is unconstrained, otherwise every ID must refer to an
// existing policy and the group's default storage_policy_id must be a member.
func (s *UpsertGroupService) validateAssignedPolicies(c *gin.Context) error {
	if s.Group == nil || s.Group.Settings == nil || len(s.Group.Settings.AssignedPolicyIDs) == 0 {
		return nil
	}

	dep := dependency.FromContext(c)
	seen := make(map[int]bool, len(s.Group.Settings.AssignedPolicyIDs))
	for _, id := range s.Group.Settings.AssignedPolicyIDs {
		if id <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "invalid assigned storage policy id", nil)
		}
		if _, err := dep.StoragePolicyClient().GetPolicyByID(c, id); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "unknown storage policy in assigned set", err)
		}
		seen[id] = true
	}

	if !seen[s.Group.StoragePolicyID] {
		return serializer.NewError(serializer.CodeParamErr, "default storage policy is not in assigned set", nil)
	}
	return nil
}

// normalizeAndValidateBindings makes sure every directory -> storage policy
// binding points to an existing policy and stores paths in a canonical form.
func (s *UpsertGroupService) normalizeAndValidateBindings(c *gin.Context) error {
	if s.Group.Settings == nil || len(s.Group.Settings.StoragePolicyBindings) == 0 {
		return nil
	}

	dep := dependency.FromContext(c)
	normalized := make(map[string]int, len(s.Group.Settings.StoragePolicyBindings))
	for p, policyID := range s.Group.Settings.StoragePolicyBindings {
		if policyID <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "invalid storage policy id in directory binding", nil)
		}
		if _, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "unknown storage policy in directory binding", err)
		}
		normalized[inventory.NormalizePolicyBindingPath(p)] = policyID
	}
	s.Group.Settings.StoragePolicyBindings = normalized
	return nil
}

func (s *UpsertGroupService) Update(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	if err := s.validateAssignedPolicies(c); err != nil {
		return nil, err
	}
	if err := s.normalizeAndValidateBindings(c); err != nil {
		return nil, err
	}

	if s.Group.ID == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID is required", nil)
	}

	// Initial admin group have to be admin
	if s.Group.ID == 1 && !s.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, serializer.NewError(serializer.CodeParamErr, "Initial admin group have to be admin", nil)
	}

	group, err := groupClient.Upsert(c, s.Group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update group", err)
	}

	service := &SingleGroupService{ID: group.ID}
	return service.Get(c)
}

func (s *UpsertGroupService) Create(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	if err := s.validateAssignedPolicies(c); err != nil {
		return nil, err
	}
	if err := s.normalizeAndValidateBindings(c); err != nil {
		return nil, err
	}

	if s.Group.ID > 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID must be 0", nil)
	}

	group, err := groupClient.Upsert(c, s.Group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create group", err)
	}

	service := &SingleGroupService{ID: group.ID}
	return service.Get(c)
}
