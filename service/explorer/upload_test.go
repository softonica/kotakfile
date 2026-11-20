package explorer

import (
	"errors"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

func TestUploadServicePolicyAuthorization(t *testing.T) {
	encoder, err := hashid.New("test-policy-authorization-salt")
	if err != nil {
		t.Fatalf("failed to create hashid encoder: %v", err)
	}

	tests := []struct {
		name     string
		policyID string
		assigned []int
		noGroup  bool
		wantErr  bool
	}{
		{
			name:     "unassigned policy is rejected",
			policyID: hashid.EncodePolicyID(encoder, 4),
			assigned: []int{2, 3},
			wantErr:  true,
		},
		{
			name:     "assigned policy is accepted",
			policyID: hashid.EncodePolicyID(encoder, 3),
			assigned: []int{2, 3},
		},
		{
			name:     "empty assigned set allows any policy",
			policyID: hashid.EncodePolicyID(encoder, 4),
			assigned: []int{},
		},
		{
			name:     "nil assigned set allows any policy (legacy)",
			policyID: hashid.EncodePolicyID(encoder, 4),
			assigned: nil,
		},
		{
			name:     "missing group edge skips check",
			policyID: hashid.EncodePolicyID(encoder, 4),
			assigned: []int{2, 3},
			noGroup:  true,
		},
		{
			name:     "no client policy id skips check",
			policyID: "",
			assigned: []int{2, 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := &ent.User{}
			if !test.noGroup {
				user.Edges.Group = &ent.Group{
					Settings: &types.GroupSetting{AssignedPolicyIDs: test.assigned},
				}
			}
			service := &CreateUploadSessionService{PolicyID: test.policyID}

			_, err := service.resolveClientPolicyID(user, encoder)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected policy authorization to fail")
				}
				var appErr serializer.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected AppError, got %T", err)
				}
				if appErr.Code != serializer.CodeParamErr {
					t.Fatalf("unexpected error code: %d", appErr.Code)
				}
				if appErr.Msg != "policy not assigned to your group" {
					t.Fatalf("unexpected error message: %q", appErr.Msg)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected policy authorization to pass: %v", err)
			}
		})
	}
}
