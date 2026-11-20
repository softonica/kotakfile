package inventory

import (
	"context"
	"encoding/gob"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/storagepolicy"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/samber/lo"
)

const (
	// StoragePolicyCacheKey is the cache key of storage policy.
	StoragePolicyCacheKey = "storage_policy_"
)

func init() {
	gob.Register(ent.StoragePolicy{})
	gob.Register([]ent.StoragePolicy{})
}

type (
	LoadStoragePolicyGroup struct{}
	SkipStoragePolicyCache struct{}

	StoragePolicyClient interface {
		TxOperator
		// GetByGroup returns the default storage policy of the group.
		GetByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error)
		// GetByPath resolves the storage policy for the given owner-view directory
		// path through the cascade: user binding, group binding, user preferred
		// policy, then the group's default policy.
		GetByPath(ctx context.Context, group *ent.Group, user *ent.User, p string) (*ent.StoragePolicy, error)
		// GetPolicyByID returns the storage policy by id.
		GetPolicyByID(ctx context.Context, id int) (*ent.StoragePolicy, error)
		// UpdateAccessKey updates the access key of the storage policy. It also clear related cache in KV.
		UpdateAccessKey(ctx context.Context, policy *ent.StoragePolicy, token string) error
		// ListPolicyByType returns the storage policies by type.
		ListPolicyByType(ctx context.Context, t types.PolicyType) ([]*ent.StoragePolicy, error)
		// ListPolicies returns the storage policies with pagination.
		ListPolicies(ctx context.Context, args *ListPolicyParameters) (*ListPolicyResult, error)
		// Upsert upserts the storage policy.
		Upsert(ctx context.Context, policy *ent.StoragePolicy) (*ent.StoragePolicy, error)
		// Delete deletes the storage policy.
		Delete(ctx context.Context, policy *ent.StoragePolicy) error
	}

	ListPolicyParameters struct {
		*PaginationArgs
		Type types.PolicyType
	}

	ListPolicyResult struct {
		*PaginationResults
		Policies []*ent.StoragePolicy
	}
)

// NewStoragePolicyClient returns a new StoragePolicyClient.
func NewStoragePolicyClient(client *ent.Client, cache cache.Driver) StoragePolicyClient {
	return &storagePolicyClient{client: client, cache: cache}
}

type storagePolicyClient struct {
	client *ent.Client
	cache  cache.Driver
}

func (c *storagePolicyClient) SetClient(newClient *ent.Client) TxOperator {
	return &storagePolicyClient{client: newClient, cache: c.cache}
}

func (c *storagePolicyClient) GetClient() *ent.Client {
	return c.client
}

func (c *storagePolicyClient) Delete(ctx context.Context, policy *ent.StoragePolicy) error {
	if err := c.client.StoragePolicy.DeleteOne(policy).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete storage policy: %w", err)
	}

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return fmt.Errorf("failed to clear storage policy cache: %w", err)
	}
	return nil
}

func (c *storagePolicyClient) Upsert(ctx context.Context, policy *ent.StoragePolicy) (*ent.StoragePolicy, error) {
	var nodeId *int
	if policy.NodeID != 0 {
		nodeId = &policy.NodeID
	}
	if policy.ID == 0 {
		p, err := c.client.StoragePolicy.
			Create().
			SetName(policy.Name).
			SetType(policy.Type).
			SetServer(policy.Server).
			SetBucketName(policy.BucketName).
			SetIsPrivate(policy.IsPrivate).
			SetAccessKey(policy.AccessKey).
			SetSecretKey(policy.SecretKey).
			SetMaxSize(policy.MaxSize).
			SetDirNameRule(policy.DirNameRule).
			SetFileNameRule(policy.FileNameRule).
			SetSettings(policy.Settings).
			SetNillableNodeID(nodeId).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage policy: %w", err)
		}
		return p, nil
	}

	updateQuery := c.client.StoragePolicy.UpdateOne(policy).
		SetName(policy.Name).
		SetType(policy.Type).
		SetServer(policy.Server).
		SetBucketName(policy.BucketName).
		SetIsPrivate(policy.IsPrivate).
		SetSecretKey(policy.SecretKey).
		SetMaxSize(policy.MaxSize).
		SetDirNameRule(policy.DirNameRule).
		SetFileNameRule(policy.FileNameRule).
		SetSettings(policy.Settings).
		SetNillableNodeID(nodeId)

	if policy.Type != types.PolicyTypeOd {
		updateQuery.SetAccessKey(policy.AccessKey)
	}

	p, err := updateQuery.Save(ctx)

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return nil, fmt.Errorf("failed to clear storage policy cache: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update storage policy: %w", err)
	}
	return p, nil

}

func (c *storagePolicyClient) GetByGroup(ctx context.Context, group *ent.Group) (*ent.StoragePolicy, error) {
	val, skipCache := ctx.Value(SkipStoragePolicyCache{}).(bool)
	skipCache = skipCache && val

	res, err := withStoragePolicyEagerLoading(ctx, c.client.Group.QueryStoragePolicies(group)).WithNode().First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get storage policies: %w", err)
	}

	return res, nil
}

// GetByPath resolves the storage policy for a directory path through the
// following cascade, most specific first:
//
//  1. user-level directory binding
//  2. group-level directory binding
//  3. user's preferred policy (when permitted by the group's assigned set)
//  4. group's default policy
//
// A binding on a parent directory applies to all of its children; the most
// specific (longest) matching prefix wins. "/" can be used as a catch-all
// binding. Each level that fails to resolve falls through to the next.
func (c *storagePolicyClient) GetByPath(ctx context.Context, group *ent.Group, user *ent.User, p string) (*ent.StoragePolicy, error) {
	// Level 1: user-level directory binding.
	if user != nil && user.Settings != nil && len(user.Settings.StoragePolicyBindings) > 0 {
		if id, ok := MatchPolicyBinding(user.Settings.StoragePolicyBindings, p); ok && id > 0 {
			policy, err := c.GetPolicyByID(ctx, id)
			if err == nil {
				return policy, nil
			}
		}
	}

	// Level 2: group-level directory binding.
	if group != nil && group.Settings != nil && len(group.Settings.StoragePolicyBindings) > 0 {
		if id, ok := MatchPolicyBinding(group.Settings.StoragePolicyBindings, p); ok && id > 0 {
			policy, err := c.GetPolicyByID(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("get bound storage policy %d: %w", id, err)
			}
			return policy, nil
		}
	}

	// Level 3: user's preferred policy. An empty group assigned set leaves the
	// user unconstrained; otherwise the preference must be a member of it.
	if user != nil && user.Settings != nil && user.Settings.PreferredPolicy > 0 {
		preferred := user.Settings.PreferredPolicy
		var assigned []int
		if group != nil && group.Settings != nil {
			assigned = group.Settings.AssignedPolicyIDs
		}
		if len(assigned) == 0 || lo.Contains(assigned, preferred) {
			policy, err := c.GetPolicyByID(ctx, preferred)
			if err == nil {
				return policy, nil
			}
		}
	}

	// Level 4: group's default policy.
	return c.GetByGroup(ctx, group)
}

// MatchPolicyBinding returns the policy ID of the longest directory prefix
// matching p. Prefixes are normalized so "/videos", "videos/" and "/videos/"
// are treated identically.
func MatchPolicyBinding(bindings map[string]int, p string) (int, bool) {
	p = NormalizePolicyBindingPath(p)

	var (
		best     string
		policyID int
		found    bool
	)
	for prefix, id := range bindings {
		np := NormalizePolicyBindingPath(prefix)
		if np == "/" || p == np || strings.HasPrefix(p, np+"/") {
			if !found || len(np) > len(best) {
				best = np
				policyID = id
				found = true
			}
		}
	}

	return policyID, found
}

// NormalizePolicyBindingPath cleans a directory path used in policy bindings.
func NormalizePolicyBindingPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// GetPolicyByID returns the storage policy by id.
func (c *storagePolicyClient) GetPolicyByID(ctx context.Context, id int) (*ent.StoragePolicy, error) {
	val, skipCache := ctx.Value(SkipStoragePolicyCache{}).(bool)
	skipCache = skipCache && val

	// Try to read from cache
	if c.cache != nil && !skipCache {
		if res, ok := c.cache.Get(StoragePolicyCacheKey + strconv.Itoa(id)); ok {
			cached := res.(ent.StoragePolicy)
			return &cached, nil
		}
	}

	res, err := withStoragePolicyEagerLoading(ctx, c.client.StoragePolicy.Query().Where(storagepolicy.ID(id))).WithNode().First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get storage policy: %w", err)
	}

	// Write to cache
	if c.cache != nil && !skipCache {
		_ = c.cache.Set(StoragePolicyCacheKey+strconv.Itoa(id), *res, -1)
	}

	return res, nil
}

func (c *storagePolicyClient) ListPolicyByType(ctx context.Context, t types.PolicyType) ([]*ent.StoragePolicy, error) {
	policies, err := c.client.StoragePolicy.Query().Where(storagepolicy.TypeEQ(string(t))).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage policies: %w", err)
	}

	return policies, nil
}

func (c *storagePolicyClient) UpdateAccessKey(ctx context.Context, policy *ent.StoragePolicy, token string) error {
	_, err := c.client.StoragePolicy.UpdateOne(policy).SetAccessKey(token).Save(ctx)
	if err != nil {
		return fmt.Errorf("faield to update access key in DB: %w", err)
	}

	// Clear cache
	if err := c.cache.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil {
		return fmt.Errorf("failed to clear storage policy cache: %w", err)
	}

	return nil
}

func (c *storagePolicyClient) ListPolicies(ctx context.Context, args *ListPolicyParameters) (*ListPolicyResult, error) {
	query := c.client.StoragePolicy.Query().WithNode()
	if args.Type != "" {
		query = query.Where(storagepolicy.TypeEQ(string(args.Type)))
	}
	query.Order(getStoragePolicyOrderOption(args)...)

	// Count total items
	total, err := query.Clone().
		Count(ctx)
	if err != nil {
		return nil, err
	}

	policies, err := withStoragePolicyEagerLoading(ctx, query).Limit(args.PageSize).Offset(args.Page * args.PageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListPolicyResult{
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   args.PageSize,
		},
		Policies: policies,
	}, nil
}

func getStoragePolicyOrderOption(args *ListPolicyParameters) []storagepolicy.OrderOption {
	orderTerm := getOrderTerm(args.Order)
	switch args.OrderBy {
	case storagepolicy.FieldUpdatedAt:
		return []storagepolicy.OrderOption{storagepolicy.ByUpdatedAt(orderTerm), storagepolicy.ByID(orderTerm)}
	default:
		return []storagepolicy.OrderOption{storagepolicy.ByID(orderTerm)}
	}
}

func withStoragePolicyEagerLoading(ctx context.Context, query *ent.StoragePolicyQuery) *ent.StoragePolicyQuery {
	if _, ok := ctx.Value(LoadStoragePolicyGroup{}).(bool); ok {
		query = query.WithGroups(func(gq *ent.GroupQuery) {
			withGroupEagerLoading(ctx, gq)
		})
	}
	return query
}
