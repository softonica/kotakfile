package user

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// userTestClient records calls to SaveSettings so tests can assert that valid
// policy preferences are persisted.
type userTestClient struct {
	inventory.UserClient
	saveCalls int
}

func (c *userTestClient) SaveSettings(_ context.Context, _ *ent.User) error {
	c.saveCalls++
	return nil
}

type userTestDep struct {
	dependency.Dep
	client *userTestClient
}

func (d userTestDep) UserClient() inventory.UserClient { return d.client }

func patchSettingTestContext(assigned []int, settings *types.UserSetting) (*gin.Context, *ent.User, *userTestClient) {
	gin.SetMode(gin.TestMode)
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.ContextWithFallback = true

	client := &userTestClient{}
	ctx := context.WithValue(context.Background(), dependency.DepCtx{}, userTestDep{client: client})

	u := &ent.User{
		ID:       1,
		Settings: settings,
		Edges: ent.UserEdges{Group: &ent.Group{
			ID:       1,
			Settings: &types.GroupSetting{AssignedPolicyIDs: assigned},
		}},
	}
	ctx = context.WithValue(ctx, inventory.UserCtx{}, u)
	c.Request = httptest.NewRequest("PATCH", "/user/setting", nil).WithContext(ctx)
	return c, u, client
}

func TestPatchUserSettingPolicyValidation(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	t.Run("preferred policy outside assigned set is rejected", func(t *testing.T) {
		c, u, client := patchSettingTestContext([]int{2, 3}, &types.UserSetting{})
		s := &PatchUserSetting{PreferredPolicy: intPtr(4)}

		err := s.Patch(c)

		requireAppParamErr(t, err)
		require.Zero(t, client.saveCalls, "invalid settings must not be persisted")
		require.Zero(t, u.Settings.PreferredPolicy)
	})

	t.Run("preferred policy inside assigned set is saved", func(t *testing.T) {
		c, u, client := patchSettingTestContext([]int{2, 3}, &types.UserSetting{})
		s := &PatchUserSetting{PreferredPolicy: intPtr(3)}

		require.NoError(t, s.Patch(c))
		require.Equal(t, 1, client.saveCalls)
		require.Equal(t, 3, u.Settings.PreferredPolicy)
	})

	t.Run("binding outside assigned set is rejected", func(t *testing.T) {
		c, u, client := patchSettingTestContext([]int{2, 3}, &types.UserSetting{})
		s := &PatchUserSetting{StoragePolicyBindings: map[string]int{"/videos": 4}}

		err := s.Patch(c)

		requireAppParamErr(t, err)
		require.Zero(t, client.saveCalls, "invalid settings must not be persisted")
		require.Empty(t, u.Settings.StoragePolicyBindings)
	})

	t.Run("binding inside assigned set is normalized and saved", func(t *testing.T) {
		c, u, client := patchSettingTestContext([]int{2, 3}, &types.UserSetting{})
		s := &PatchUserSetting{StoragePolicyBindings: map[string]int{"videos/": 3}}

		require.NoError(t, s.Patch(c))
		require.Equal(t, 1, client.saveCalls)
		require.Equal(t, map[string]int{"/videos": 3}, u.Settings.StoragePolicyBindings)
	})

	t.Run("empty assigned set accepts any policy", func(t *testing.T) {
		c, u, client := patchSettingTestContext(nil, &types.UserSetting{})
		s := &PatchUserSetting{
			PreferredPolicy:       intPtr(4),
			StoragePolicyBindings: map[string]int{"/videos": 4},
		}

		require.NoError(t, s.Patch(c))
		require.Equal(t, 1, client.saveCalls)
		require.Equal(t, 4, u.Settings.PreferredPolicy)
		require.Equal(t, map[string]int{"/videos": 4}, u.Settings.StoragePolicyBindings)
	})
}

func requireAppParamErr(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var appErr serializer.AppError
	require.True(t, errors.As(err, &appErr), "expected serializer.AppError, got %T", err)
	require.Equal(t, serializer.CodeParamErr, appErr.Code)
}
