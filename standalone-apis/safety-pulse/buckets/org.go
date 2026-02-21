package buckets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Org roles (hierarchy: owner > admin > manager > member > guest)
const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleMember  = "member"
	RoleGuest   = "guest"
)

// Resource visibility
const (
	VisibilityPrivate    = "private"     // creator only
	VisibilityInviteOnly = "invite-only" // creator + explicit shares
	VisibilityTeam       = "team"        // members+ (no guests)
	VisibilityOrgWide    = "org-wide"    // everyone in org
)

// OrgConfig loaded from org.json
type OrgConfig struct {
	Resources map[string]ResourceConfig `json:"resources"`
}

type ResourceConfig struct {
	Write             string `json:"write"`              // minimum role to create/edit
	DefaultVisibility string `json:"default_visibility"` // default visibility for new items
}

var (
	orgConfig     *OrgConfig
	orgConfigOnce sync.Once
	orgConfigFile = "org.json"
)

// LoadOrgConfig loads org.json (called once, cached)
func LoadOrgConfig() *OrgConfig {
	orgConfigOnce.Do(func() {
		orgConfig = &OrgConfig{
			Resources: make(map[string]ResourceConfig),
		}

		data, err := os.ReadFile(orgConfigFile)
		if err != nil {
			// No config file = use defaults
			return
		}

		json.Unmarshal(data, orgConfig)
	})
	return orgConfig
}

// GetResourceConfig returns config for a resource, with defaults
func GetResourceConfig(resourceName string) ResourceConfig {
	cfg := LoadOrgConfig()
	if rc, ok := cfg.Resources[resourceName]; ok {
		// Fill in defaults if not specified
		if rc.Write == "" {
			rc.Write = RoleMember
		}
		if rc.DefaultVisibility == "" {
			rc.DefaultVisibility = VisibilityTeam
		}
		return rc
	}
	// Default for unknown resources
	return ResourceConfig{
		Write:             RoleMember,
		DefaultVisibility: VisibilityTeam,
	}
}

// OrgMember represents a user's membership in an org
type OrgMember struct {
	ID        string    `firestore:"id" json:"id"`
	OrgID     string    `firestore:"org_id" json:"org_id"`
	UserID    string    `firestore:"user_id" json:"user_id"`
	Role      string    `firestore:"role" json:"role"`
	InvitedBy string    `firestore:"invited_by" json:"invited_by"`
	JoinedAt  time.Time `firestore:"joined_at" json:"joined_at"`
}

// OrgSharing represents a share within an org
type OrgSharing struct {
	ID             string    `firestore:"id" json:"id"`
	OrgID          string    `firestore:"org_id" json:"org_id"`
	ResourceType   string    `firestore:"resource_type" json:"resource_type"`
	ResourceID     string    `firestore:"resource_id" json:"resource_id"`
	SharedWithUser string    `firestore:"shared_with_user,omitempty" json:"shared_with_user,omitempty"`
	SharedWithRole string    `firestore:"shared_with_role,omitempty" json:"shared_with_role,omitempty"`
	Access         string    `firestore:"access" json:"access"` // read or write
	SharedBy       string    `firestore:"shared_by" json:"shared_by"`
	CreatedAt      time.Time `firestore:"created_at" json:"created_at"`
}

type OrgBucketImpl struct {
	client            *firestore.Client
	defaultVisibility string
}

func NewOrgBucket(client *firestore.Client, defaultVisibility string) *OrgBucketImpl {
	if defaultVisibility == "" {
		defaultVisibility = VisibilityTeam
	}
	return &OrgBucketImpl{client: client, defaultVisibility: defaultVisibility}
}

// Create a resource in org bucket
func (b *OrgBucketImpl) Create(ctx context.Context, orgID, bucketName string, data map[string]interface{}) (string, error) {
	userID, role, err := b.requireOrgMember(ctx, orgID)
	if err != nil {
		return "", err
	}

	// Check write permission from config
	resourceCfg := GetResourceConfig(bucketName)
	if roleLevel(role) < roleLevel(resourceCfg.Write) {
		return "", fmt.Errorf("forbidden: %s requires %s+ to create", bucketName, resourceCfg.Write)
	}

	id := uuid.New().String()
	data["id"] = id
	data["org_id"] = orgID // ENFORCED - can't override
	data["created_by"] = userID
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()

	// Set visibility from config if not provided
	if _, ok := data["visibility"]; !ok {
		data["visibility"] = resourceCfg.DefaultVisibility
	}

	collection := fmt.Sprintf("org-%s", bucketName)
	_, err = b.client.Collection(collection).Doc(id).Set(ctx, data)

	return id, err
}

// Get a resource from org bucket
func (b *OrgBucketImpl) Get(ctx context.Context, orgID, bucketName, id string) (map[string]interface{}, error) {
	userID, role, err := b.requireOrgMember(ctx, orgID)
	if err != nil {
		return nil, err
	}

	collection := fmt.Sprintf("org-%s", bucketName)
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, errors.New("not found")
		}
		return nil, err
	}

	data := doc.Data()

	// Verify org boundary
	if data["org_id"] != orgID {
		return nil, errors.New("not found") // Don't leak that it exists
	}

	// Check visibility
	if !b.canAccess(ctx, orgID, bucketName, id, data, userID, role, "read") {
		return nil, errors.New("forbidden")
	}

	return data, nil
}

// Update a resource in org bucket
func (b *OrgBucketImpl) Update(ctx context.Context, orgID, bucketName, id string, data map[string]interface{}) error {
	userID, role, err := b.requireOrgMember(ctx, orgID)
	if err != nil {
		return err
	}

	collection := fmt.Sprintf("org-%s", bucketName)
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errors.New("not found")
		}
		return err
	}

	existing := doc.Data()

	// Verify org boundary
	if existing["org_id"] != orgID {
		return errors.New("not found")
	}

	// Check write access
	if !b.canAccess(ctx, orgID, bucketName, id, existing, userID, role, "write") {
		return errors.New("forbidden")
	}

	// Preserve org fields
	data["id"] = id
	data["org_id"] = orgID
	data["created_by"] = existing["created_by"]
	data["created_at"] = existing["created_at"]
	data["updated_at"] = time.Now()

	// Only creator/admin can change visibility
	if data["visibility"] != existing["visibility"] {
		if existing["created_by"] != userID && roleLevel(role) < roleLevel(RoleAdmin) {
			data["visibility"] = existing["visibility"] // revert
		}
	}

	_, err = b.client.Collection(collection).Doc(id).Set(ctx, data)
	return err
}

// Delete a resource from org bucket
func (b *OrgBucketImpl) Delete(ctx context.Context, orgID, bucketName, id string) error {
	userID, role, err := b.requireOrgMember(ctx, orgID)
	if err != nil {
		return err
	}

	collection := fmt.Sprintf("org-%s", bucketName)
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errors.New("not found")
		}
		return err
	}

	existing := doc.Data()

	// Verify org boundary
	if existing["org_id"] != orgID {
		return errors.New("not found")
	}

	// Only creator or admin+ can delete
	if existing["created_by"] != userID && roleLevel(role) < roleLevel(RoleAdmin) {
		return errors.New("forbidden: only creator or admin can delete")
	}

	_, err = b.client.Collection(collection).Doc(id).Delete(ctx)
	return err
}

// List resources in org bucket visible to user
func (b *OrgBucketImpl) List(ctx context.Context, orgID, bucketName string) ([]map[string]interface{}, error) {
	userID, role, err := b.requireOrgMember(ctx, orgID)
	if err != nil {
		return nil, err
	}

	collection := fmt.Sprintf("org-%s", bucketName)
	iter := b.client.Collection(collection).Where("org_id", "==", orgID).Documents(ctx)

	var results []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		data := doc.Data()
		if b.canAccess(ctx, orgID, bucketName, data["id"].(string), data, userID, role, "read") {
			results = append(results, data)
		}
	}

	return results, nil
}

// canAccess checks if user can access resource with given access level
func (b *OrgBucketImpl) canAccess(ctx context.Context, orgID, bucketName, resourceID string, data map[string]interface{}, userID, role, accessNeeded string) bool {
	visibility := data["visibility"].(string)
	createdBy := data["created_by"].(string)

	// Creator always has access
	if createdBy == userID {
		return true
	}

	// Admin+ has full access
	if roleLevel(role) >= roleLevel(RoleAdmin) {
		return true
	}

	switch visibility {
	case VisibilityOrgWide:
		return true // everyone in org

	case VisibilityTeam:
		return roleLevel(role) >= roleLevel(RoleMember) // no guests

	case VisibilityInviteOnly:
		return b.hasOrgSharing(ctx, orgID, bucketName, resourceID, userID, role, accessNeeded)

	case VisibilityPrivate:
		return false // only creator (already checked above)

	default:
		return false
	}
}

// hasOrgSharing checks org-sharings table
func (b *OrgBucketImpl) hasOrgSharing(ctx context.Context, orgID, bucketName, resourceID, userID, userRole, accessNeeded string) bool {
	iter := b.client.Collection("org-sharings").
		Where("org_id", "==", orgID).
		Where("resource_type", "==", bucketName).
		Where("resource_id", "==", resourceID).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false
		}

		var sharing OrgSharing
		doc.DataTo(&sharing)

		// Check user match
		if sharing.SharedWithUser == userID {
			if accessNeeded == "read" || sharing.Access == "write" {
				return true
			}
		}

		// Check role match (shared with role means that role and above)
		if sharing.SharedWithRole != "" && roleLevel(userRole) >= roleLevel(sharing.SharedWithRole) {
			if accessNeeded == "read" || sharing.Access == "write" {
				return true
			}
		}
	}

	return false
}

// requireOrgMember verifies user is member of org, returns userID and role
func (b *OrgBucketImpl) requireOrgMember(ctx context.Context, orgID string) (string, string, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return "", "", errors.New("unauthorized")
	}

	// Look up membership
	iter := b.client.Collection("org-members").
		Where("org_id", "==", orgID).
		Where("user_id", "==", userID).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return "", "", errors.New("forbidden: not a member of this org")
	}
	if err != nil {
		return "", "", err
	}

	var member OrgMember
	doc.DataTo(&member)

	return userID, member.Role, nil
}

func roleLevel(role string) int {
	switch role {
	case RoleOwner:
		return 5
	case RoleAdmin:
		return 4
	case RoleManager:
		return 3
	case RoleMember:
		return 2
	case RoleGuest:
		return 1
	default:
		return 0
	}
}
