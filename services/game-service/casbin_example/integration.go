// Integration: Casbin with Org Buckets
//
// This shows how Casbin would replace our current hardcoded checks

package main

import (
	"context"
	"errors"

	"github.com/casbin/casbin/v2"
)

// AuthzService wraps Casbin enforcer
type AuthzService struct {
	enforcer *casbin.Enforcer
}

func NewAuthzService(modelPath, policyPath string) (*AuthzService, error) {
	e, err := casbin.NewEnforcer(modelPath, policyPath)
	if err != nil {
		return nil, err
	}
	return &AuthzService{enforcer: e}, nil
}

// CanDo checks if user can perform action on resource in org
func (a *AuthzService) CanDo(userID, orgID, resource, action string) bool {
	allowed, _ := a.enforcer.Enforce(userID, orgID, resource, action)
	return allowed
}

// AddMember adds user to org with role
func (a *AuthzService) AddMember(userID, role, orgID string) error {
	_, err := a.enforcer.AddGroupingPolicy(userID, role, orgID)
	return err
}

// RemoveMember removes user from org
func (a *AuthzService) RemoveMember(userID, role, orgID string) error {
	_, err := a.enforcer.RemoveGroupingPolicy(userID, role, orgID)
	return err
}

// GetUserRole gets user's role in org
func (a *AuthzService) GetUserRole(userID, orgID string) string {
	roles := a.enforcer.GetRolesForUserInDomain(userID, orgID)
	if len(roles) > 0 {
		return roles[0]
	}
	return ""
}

// ============================================================
// How org bucket would use it
// ============================================================

type OrgBucketWithCasbin struct {
	authz *AuthzService
	// ... firestore client etc
}

func (b *OrgBucketWithCasbin) Create(ctx context.Context, orgID, bucketName string, data map[string]interface{}) (string, error) {
	userID := ctx.Value("user_id").(string)

	// Simple check - replaces hardcoded role checks
	if !b.authz.CanDo(userID, orgID, bucketName, "create") {
		return "", errors.New("forbidden: cannot create " + bucketName)
	}

	// ... create logic
	return "new-id", nil
}

func (b *OrgBucketWithCasbin) Approve(ctx context.Context, orgID, bucketName, itemID string) error {
	userID := ctx.Value("user_id").(string)

	// Action-specific check
	if !b.authz.CanDo(userID, orgID, bucketName, "approve") {
		return errors.New("forbidden: cannot approve")
	}

	// ... update item state to "approved"
	return nil
}

func (b *OrgBucketWithCasbin) Publish(ctx context.Context, orgID, bucketName, itemID string) error {
	userID := ctx.Value("user_id").(string)

	if !b.authz.CanDo(userID, orgID, bucketName, "publish") {
		return errors.New("forbidden: cannot publish")
	}

	// ... update item state to "published"
	return nil
}

func (b *OrgBucketWithCasbin) Delete(ctx context.Context, orgID, bucketName, itemID string) error {
	userID := ctx.Value("user_id").(string)

	if !b.authz.CanDo(userID, orgID, bucketName, "delete") {
		return errors.New("forbidden: cannot delete")
	}

	// ... delete logic
	return nil
}

// ============================================================
// Policy in code (alternative to CSV file)
// ============================================================

func SetupPolicies(authz *AuthzService) {
	e := authz.enforcer

	// Role hierarchy (applies to all orgs)
	e.AddNamedGroupingPolicy("g", "member", "guest", "*")
	e.AddNamedGroupingPolicy("g", "manager", "member", "*")
	e.AddNamedGroupingPolicy("g", "admin", "manager", "*")
	e.AddNamedGroupingPolicy("g", "owner", "admin", "*")

	// Notes permissions
	e.AddPolicy("member", "*", "notes", "create")
	e.AddPolicy("member", "*", "notes", "edit_own")
	e.AddPolicy("manager", "*", "notes", "approve")
	e.AddPolicy("admin", "*", "notes", "publish")
	e.AddPolicy("admin", "*", "notes", "delete")
	e.AddPolicy("guest", "*", "notes", "view_published")

	// Projects permissions
	e.AddPolicy("manager", "*", "projects", "create")
	e.AddPolicy("member", "*", "projects", "view")
	e.AddPolicy("admin", "*", "projects", "delete")

	// Announcements
	e.AddPolicy("admin", "*", "announcements", "create")
	e.AddPolicy("guest", "*", "announcements", "view")
}

// ============================================================
// Load from our org.json format
// ============================================================

/*
{
  "resources": {
    "notes": {
      "actions": {
        "create": "member",
        "approve": "manager",
        "publish": "admin",
        "delete": "admin"
      }
    }
  }
}

func LoadFromOrgJSON(authz *AuthzService, configPath string) {
    // Read org.json
    // For each resource -> actions -> role:
    //   e.AddPolicy(role, "*", resource, action)
}
*/
