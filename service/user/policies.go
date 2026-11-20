package user

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// userPoliciesPageSize is used when a user's group has no assigned policy set,
// so every policy is returned. It is intentionally large to avoid pagination.
const userPoliciesPageSize = 1000000

type (
	// ListUserPoliciesResponse is the user-facing list of policies they may select.
	ListUserPoliciesResponse struct {
		Policies          []*ent.StoragePolicy `json:"policies"`
		AssignedPolicyIDs []int                `json:"assigned_policy_ids"`
	}

	// ListUserPoliciesService lists storage policies available to the current user.
	ListUserPoliciesService struct{}
)

// List returns the policies assigned to the current user's group. If the group
// has no assigned set, all policies are returned and assigned_policy_ids is empty.
func (service *ListUserPoliciesService) List(c *gin.Context) (*ListUserPoliciesResponse, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)

	var assigned []int
	if u != nil && u.Edges.Group != nil && u.Edges.Group.Settings != nil {
		assigned = u.Edges.Group.Settings.AssignedPolicyIDs
	}

	client := dep.StoragePolicyClient()
	res := &ListUserPoliciesResponse{}

	if len(assigned) > 0 {
		for _, id := range assigned {
			policy, err := client.GetPolicyByID(c, id)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeDBError, "Failed to get assigned storage policy", err)
			}
			res.Policies = append(res.Policies, policy)
		}
		res.AssignedPolicyIDs = assigned
		return res, nil
	}

	listed, err := client.ListPolicies(c, &inventory.ListPolicyParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     0,
			PageSize: userPoliciesPageSize,
		},
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list storage policies", err)
	}
	res.Policies = listed.Policies
	return res, nil
}
