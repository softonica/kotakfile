package user

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// policiesTestClient records which lookup path ListPoliciesService takes.
type policiesTestClient struct {
	inventory.StoragePolicyClient
	byID       map[int]*ent.StoragePolicy
	all        []*ent.StoragePolicy
	getCalls   []int
	listCalled bool
	pageSize   int
}

func (c *policiesTestClient) GetPolicyByID(_ context.Context, id int) (*ent.StoragePolicy, error) {
	c.getCalls = append(c.getCalls, id)
	if p, ok := c.byID[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("policy %d not found", id)
}

func (c *policiesTestClient) ListPolicies(_ context.Context, args *inventory.ListPolicyParameters) (*inventory.ListPolicyResult, error) {
	c.listCalled = true
	c.pageSize = args.PageSize
	return &inventory.ListPolicyResult{
		PaginationResults: &inventory.PaginationResults{},
		Policies:          c.all,
	}, nil
}

type policiesTestDep struct {
	dependency.Dep
	client *policiesTestClient
}

func (d policiesTestDep) StoragePolicyClient() inventory.StoragePolicyClient { return d.client }

func policiesTestContext(assigned []int, client *policiesTestClient) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.ContextWithFallback = true

	ctx := context.WithValue(context.Background(), dependency.DepCtx{}, policiesTestDep{client: client})
	u := &ent.User{
		ID: 1,
		Edges: ent.UserEdges{Group: &ent.Group{
			ID:       1,
			Settings: &types.GroupSetting{AssignedPolicyIDs: assigned},
		}},
	}
	ctx = context.WithValue(ctx, inventory.UserCtx{}, u)
	c.Request = httptest.NewRequest("GET", "/user/policies", nil).WithContext(ctx)
	return c
}

func TestListUserPoliciesService(t *testing.T) {
	t.Run("assigned set restricts policies to assigned IDs", func(t *testing.T) {
		p2 := &ent.StoragePolicy{ID: 2, Name: "two"}
		p3 := &ent.StoragePolicy{ID: 3, Name: "three"}
		client := &policiesTestClient{byID: map[int]*ent.StoragePolicy{2: p2, 3: p3}}
		c := policiesTestContext([]int{2, 3}, client)

		res, err := (&ListUserPoliciesService{}).List(c)

		require.NoError(t, err)
		require.Equal(t, []int{2, 3}, res.AssignedPolicyIDs)
		require.Equal(t, []*ent.StoragePolicy{p2, p3}, res.Policies)
		require.Equal(t, []int{2, 3}, client.getCalls)
		require.False(t, client.listCalled, "assigned set must not fall back to listing all policies")
	})

	t.Run("empty assigned set returns all policies", func(t *testing.T) {
		all := []*ent.StoragePolicy{{ID: 1}, {ID: 2}, {ID: 3}}
		client := &policiesTestClient{all: all}
		c := policiesTestContext(nil, client)

		res, err := (&ListUserPoliciesService{}).List(c)

		require.NoError(t, err)
		require.Empty(t, res.AssignedPolicyIDs)
		require.Equal(t, all, res.Policies)
		require.True(t, client.listCalled, "empty assigned set must list all policies")
		require.GreaterOrEqual(t, client.pageSize, 1000)
		require.Empty(t, client.getCalls)
	})
}

func TestBuildUserSettingsIncludesPolicyFields(t *testing.T) {
	u := &ent.User{
		Settings: &types.UserSetting{
			PreferredPolicy:       3,
			StoragePolicyBindings: map[string]int{"/videos": 2},
		},
	}

	res := BuildUserSettings(u, nil, nil, nil)

	require.Equal(t, 3, res.PreferredPolicy)
	require.Equal(t, map[string]int{"/videos": 2}, res.StoragePolicyBindings)
}
