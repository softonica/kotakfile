package inventory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/stretchr/testify/require"
)

// capturingLogger records warnings emitted through logging.FromContext so the
// cascade's skip-and-warn behavior can be asserted.
type capturingLogger struct {
	warnings []string
}

func (l *capturingLogger) Panic(string, ...any) {}
func (l *capturingLogger) Error(string, ...any) {}
func (l *capturingLogger) Warning(format string, v ...any) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, v...))
}
func (l *capturingLogger) Info(string, ...any)                  {}
func (l *capturingLogger) Debug(string, ...any)                 {}
func (l *capturingLogger) CopyWithPrefix(string) logging.Logger { return l }
func (l *capturingLogger) SupportColor() bool                   { return false }

func TestNormalizePolicyBindingPath(t *testing.T) {
	require.Equal(t, "/", NormalizePolicyBindingPath(""))
	require.Equal(t, "/", NormalizePolicyBindingPath("/"))
	require.Equal(t, "/videos", NormalizePolicyBindingPath("videos/"))
	require.Equal(t, "/videos", NormalizePolicyBindingPath("/videos//"))
	require.Equal(t, "/videos/2026", NormalizePolicyBindingPath("videos/2026/"))
}

func TestMatchPolicyBindingLongestPrefix(t *testing.T) {
	bindings := map[string]int{
		"/":                   1,
		"/videos":             2,
		"/videos/2026":        3,
		"documents/contracts": 4,
	}

	tests := []struct {
		path     string
		expected int
		found    bool
	}{
		{"/", 1, true},
		{"/music/song.mp3", 1, true},
		{"/videos/clip.mp4", 2, true},
		{"/videos/2026/clip.mp4", 3, true},
		{"/videos/2025/clip.mp4", 2, true},
		{"/documents/contracts/2026.pdf", 4, true},
		{"/documents/invoice.pdf", 1, true},
	}

	for _, tt := range tests {
		id, ok := MatchPolicyBinding(bindings, tt.path)
		require.Equal(t, tt.found, ok, tt.path)
		require.Equal(t, tt.expected, id, tt.path)
	}
}

func TestMatchPolicyBindingNoMatch(t *testing.T) {
	id, ok := MatchPolicyBinding(map[string]int{"/videos": 2}, "/documents/a.txt")
	require.False(t, ok)
	require.Zero(t, id)

	// An empty binding map must not match anything.
	id, ok = MatchPolicyBinding(nil, "/videos/a.mp4")
	require.False(t, ok)
	require.Zero(t, id)
}

func TestGetByPathCascade(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "cascade.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	sc := NewStoragePolicyClient(client, nil)

	newPolicy := func(name string) *ent.StoragePolicy {
		p, err := client.StoragePolicy.Create().
			SetName(name).
			SetType(string(types.PolicyTypeLocal)).
			Save(ctx)
		require.NoError(t, err)
		return p
	}
	p1 := newPolicy("group default") // group default policy
	p2 := newPolicy("group binding") // group directory binding target
	p3 := newPolicy("user level")    // user binding / preferred target
	p4 := newPolicy("unassigned")    // preferred policy outside assigned set

	const (
		p         = "/videos/clip.mp4"
		missingID = 999999 // no policy with this ID exists
	)

	tests := []struct {
		name        string
		group       *types.GroupSetting
		user        *types.UserSetting
		userNil     bool
		nilSettings bool
		want        *ent.StoragePolicy
		wantWarning string
	}{
		{
			name:  "user binding beats group binding",
			group: &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": p2.ID}},
			user:  &types.UserSetting{StoragePolicyBindings: map[string]int{"/videos": p3.ID}},
			want:  p3,
		},
		{
			name:  "group binding beats user preferred",
			group: &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": p2.ID}},
			user:  &types.UserSetting{PreferredPolicy: p3.ID},
			want:  p2,
		},
		{
			name:  "user preferred beats group default",
			group: &types.GroupSetting{},
			user:  &types.UserSetting{PreferredPolicy: p3.ID},
			want:  p3,
		},
		{
			name:        "unassigned user preferred falls through to group default",
			group:       &types.GroupSetting{AssignedPolicyIDs: []int{p1.ID, p2.ID}},
			user:        &types.UserSetting{PreferredPolicy: p4.ID},
			want:        p1,
			wantWarning: "not assigned",
		},
		{
			name:    "nil user preserves group-only behavior",
			group:   &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": p2.ID}},
			userNil: true,
			want:    p2,
		},
		{
			name:        "nil settings are treated as empty",
			group:       &types.GroupSetting{},
			user:        &types.UserSetting{},
			nilSettings: true,
			want:        p1,
		},
		{
			name:        "invalid user binding falls through to group binding",
			group:       &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": p2.ID}},
			user:        &types.UserSetting{StoragePolicyBindings: map[string]int{"/videos": missingID}},
			want:        p2,
			wantWarning: fmt.Sprint(missingID),
		},
		{
			name:        "invalid group binding falls through to user preferred",
			group:       &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": missingID}},
			user:        &types.UserSetting{PreferredPolicy: p3.ID},
			want:        p3,
			wantWarning: fmt.Sprint(missingID),
		},
		{
			name:        "invalid group binding falls through to group default",
			group:       &types.GroupSetting{StoragePolicyBindings: map[string]int{"/videos": missingID}},
			user:        &types.UserSetting{},
			want:        p1,
			wantWarning: fmt.Sprint(missingID),
		},
		{
			name:        "invalid user preferred falls through to group default",
			group:       &types.GroupSetting{},
			user:        &types.UserSetting{PreferredPolicy: missingID},
			want:        p1,
			wantWarning: fmt.Sprint(missingID),
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, err := client.Group.Create().
				SetName(fmt.Sprintf("group-%d", i)).
				SetPermissions(&boolset.BooleanSet{}).
				SetStoragePolicyID(p1.ID).
				SetSettings(tt.group).
				Save(ctx)
			require.NoError(t, err)

			var user *ent.User
			if !tt.userNil {
				user, err = client.User.Create().
					SetEmail(fmt.Sprintf("user-%d@test.local", i)).
					SetNick("test").
					SetGroup(group).
					SetSettings(tt.user).
					Save(ctx)
				require.NoError(t, err)
			}

			if tt.nilSettings {
				group.Settings = nil
				if user != nil {
					user.Settings = nil
				}
			}

			lg := &capturingLogger{}
			testCtx := context.WithValue(ctx, logging.LoggerCtx{}, lg)
			got, err := sc.GetByPath(testCtx, group, user, p)
			require.NoError(t, err)
			require.Equal(t, tt.want.ID, got.ID)
			if tt.wantWarning != "" {
				require.Contains(t, strings.Join(lg.warnings, "\n"), tt.wantWarning)
			} else {
				require.Empty(t, lg.warnings)
			}
		})
	}
}
