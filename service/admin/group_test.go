package admin

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
)

// groupTestPolicyClient answers policy existence checks for assigned-set
// validation. Only policies 2 and 3 exist.
type groupTestPolicyClient struct {
	inventory.StoragePolicyClient
}

func (c *groupTestPolicyClient) GetPolicyByID(_ context.Context, id int) (*ent.StoragePolicy, error) {
	if id == 2 || id == 3 {
		return &ent.StoragePolicy{ID: id}, nil
	}
	return nil, errors.New("unknown storage policy")
}

type groupTestDep struct {
	dependency.Dep
	policyClient inventory.StoragePolicyClient
}

func (d groupTestDep) StoragePolicyClient() inventory.StoragePolicyClient { return d.policyClient }

func groupTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.ContextWithFallback = true
	ctx := context.WithValue(context.Background(), dependency.DepCtx{}, groupTestDep{
		policyClient: &groupTestPolicyClient{},
	})
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	return c
}

func TestUpsertGroupValidateAssignedPolicies(t *testing.T) {
	tests := []struct {
		name      string
		assigned  []int
		defaultID int
		wantErr   bool
	}{
		{name: "valid set containing default", assigned: []int{2, 3}, defaultID: 2},
		{name: "unknown policy", assigned: []int{999}, defaultID: 2, wantErr: true},
		{name: "default not in set", assigned: []int{3}, defaultID: 2, wantErr: true},
		{name: "zero id", assigned: []int{0}, defaultID: 0, wantErr: true},
		{name: "empty set", assigned: nil, defaultID: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &UpsertGroupService{Group: &ent.Group{
				StoragePolicyID: test.defaultID,
				Settings:        &types.GroupSetting{AssignedPolicyIDs: test.assigned},
			}}
			err := service.validateAssignedPolicies(groupTestContext())
			if test.wantErr {
				if err == nil {
					t.Fatal("expected assigned policy validation to fail")
				}
				var appErr serializer.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected AppError, got %T", err)
				}
				if appErr.Code != serializer.CodeParamErr {
					t.Fatalf("unexpected error code: %d", appErr.Code)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected assigned policy validation to pass: %v", err)
			}
		})
	}
}
