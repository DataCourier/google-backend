# Permission Models Comparison

Research for adopting a battle-tested authorization schema.

---

## 1. Google Zanzibar (SpiceDB)

**Used by:** Google Docs, Drive, YouTube, Maps, Cloud
**Open source:** [SpiceDB](https://github.com/authzed/spicedb), OpenFGA, Ory Keto
**Type:** Relationship-Based Access Control (ReBAC)

### Core Concept
Instead of assigning roles to users globally, you store **relationships**:
- "User X is editor of Document Y"
- "Document Y is in Folder Z"
- "User X is member of Org W"

Permissions are computed by traversing the relationship graph.

### Schema Example (SpiceDB `.zed` format)

```zed
definition user {}

definition org {
  relation owner: user
  relation admin: user
  relation member: user
  relation guest: user

  // Hierarchical: owner has admin, admin has member rights
  permission admin_access = owner + admin
  permission member_access = admin_access + member
  permission guest_access = member_access + guest
}

definition document {
  relation org: org
  relation owner: user
  relation editor: user | org#member
  relation commenter: user | org#member
  relation viewer: user | org#guest

  // Permissions with inheritance
  permission delete = owner + org->admin_access
  permission edit = delete + editor
  permission comment = edit + commenter
  permission view = comment + viewer
}
```

### Relationship Tuples (Data)
```
document:meeting-notes#org@org:acme
document:meeting-notes#owner@user:alice
document:meeting-notes#editor@org:acme#member   // all acme members can edit
document:meeting-notes#viewer@user:bob          // bob specifically can view
```

### Query
```
CheckPermission(document:meeting-notes, edit, user:charlie)
// Traverses: charlie -> acme member? -> document editor? -> true/false
```

### Pros
- Handles complex inheritance (folders, teams, sharing)
- Scales massively (Google scale)
- Auditable relationship graph
- "Share with anyone who has the link" is trivial

### Cons
- Requires separate service (SpiceDB)
- Learning curve for ReBAC thinking
- Overkill for simple apps

---

## 2. Notion

**Used by:** Notion (millions of users)
**Type:** Hierarchical RBAC with page-level sharing

### Workspace Roles

| Role | Description |
|------|-------------|
| **Workspace Owner** | Full control, billing, can delete workspace |
| **Admin** | Manage settings, members, most permissions |
| **Member** | Create pages, join teamspaces, collaborate |
| **Guest** | Page-by-page access only, no teamspaces |

### Page-Level Permissions

| Level | Can Do |
|-------|--------|
| **Full Access** | Edit, share, delete, manage permissions |
| **Can Edit** | Edit content, add sub-pages |
| **Can Comment** | View + add comments |
| **Can View** | Read only |

### Key Patterns
- **Inheritance:** Sub-pages inherit parent permissions by default
- **Override:** Can restrict or expand sub-page permissions
- **Teamspaces:** Groups of members with shared access
- **Guests:** External collaborators, page-by-page only

### Pros
- Simple mental model
- Intuitive for end users
- Good for document/page hierarchies

### Cons
- Less flexible than ReBAC
- Teamspace model adds complexity
- Guest limitations can frustrate users

---

## 3. GitHub

**Used by:** GitHub (100M+ developers)
**Type:** Hierarchical RBAC with repository focus

### Organization Roles

| Role | Description |
|------|-------------|
| **Owner** | Full org control, billing, destructive actions |
| **Member** | Default access, inherits base permissions |
| **Billing Manager** | Billing only |
| **Security Manager** | Security settings and alerts |

### Repository Roles (hierarchical)

| Role | Read | Triage | Write | Maintain | Admin |
|------|------|--------|-------|----------|-------|
| Clone/Pull | ✓ | ✓ | ✓ | ✓ | ✓ |
| Issues/Discussions | ✓ | ✓ | ✓ | ✓ | ✓ |
| Manage Issues | | ✓ | ✓ | ✓ | ✓ |
| Push | | | ✓ | ✓ | ✓ |
| Manage PRs | | | ✓ | ✓ | ✓ |
| Manage Repo Settings | | | | ✓ | ✓ |
| Delete/Transfer Repo | | | | | ✓ |

### Key Patterns
- **Base Permissions:** Org-wide default for all repos (none/read/write/admin)
- **Teams:** Groups with cascading permissions
- **Outside Collaborators:** Per-repo access for non-members
- **Custom Roles:** Enterprise only, inherit from base roles

### Pros
- Clear hierarchy
- Team-based permissions scale well
- Fine-grained for code workflows

### Cons
- Complex for large orgs
- Custom roles require Enterprise
- Repository-centric (not general purpose)

---

## Comparison Matrix

| Feature | Zanzibar/SpiceDB | Notion | GitHub |
|---------|------------------|--------|--------|
| **Model Type** | ReBAC (graph) | Hierarchical RBAC | Hierarchical RBAC |
| **Inheritance** | Graph traversal | Page tree | Team hierarchy |
| **Sharing** | Any relationship | Page-level | Repo-level |
| **Guests** | Just another relation | Limited role | Outside collaborator |
| **Custom Roles** | Schema-defined | No | Enterprise only |
| **Scale** | Massive | Large | Large |
| **Complexity** | High | Medium | Medium-High |
| **Self-hosted** | SpiceDB (Go) | N/A | N/A |

---

## Recommendation for Our Framework

### Short Term (Current)
Keep the simple JSON config we have:
```json
{
  "resources": {
    "notes": { "write": "member" },
    "projects": { "write": "manager" }
  }
}
```

This maps to **Notion's model** - simple, understandable, sufficient for most apps.

### Medium Term
If we need:
- Sharing individual items with specific users ✓ (already have)
- Folder/hierarchy permissions
- "Share with anyone with link"

Consider adopting **Zanzibar-style relationships**:
```go
// Store relationships, not just roles
type Relationship struct {
    Resource   string  // "document:123"
    Relation   string  // "editor"
    Subject    string  // "user:alice" or "org:acme#member"
}
```

### Long Term
If permission checks become a bottleneck or we need:
- Cross-service authorization
- Complex group nesting
- Audit trails for compliance

Integrate **SpiceDB** as a dedicated authorization service.

---

## Schema We Could Adopt

A Zanzibar-inspired schema that fits our org bucket model:

```zed
definition user {}

definition org {
    relation owner: user
    relation admin: user
    relation manager: user
    relation member: user
    relation guest: user

    // Role hierarchy
    permission owner_access = owner
    permission admin_access = owner_access + admin
    permission manager_access = admin_access + manager
    permission member_access = manager_access + member
    permission guest_access = member_access + guest
}

definition resource {
    relation org: org
    relation creator: user

    // Direct grants (invite-only)
    relation editor: user | org#member
    relation viewer: user | org#guest

    // Visibility setting stored as relation
    relation visibility_org_wide: org#guest      // if set, all org guests+ can view
    relation visibility_team: org#member         // if set, all org members+ can view

    // Permissions
    permission delete = creator + org->admin_access
    permission edit = delete + editor + org->manager_access
    permission view = edit + viewer + visibility_org_wide + visibility_team
}
```

This would replace our current system but maintains the same concepts:
- Roles: owner > admin > manager > member > guest
- Visibility: private, invite-only, team, org-wide
- Resource-level overrides via direct grants

---

---

## Permission Model Options (For Future Exploration)

These are patterns we can adopt depending on project needs.

### Option A: Simple Role-Based (Current)

What we have now. Roles determine access level.

```json
{
  "resources": {
    "notes": { "write": "member" },
    "projects": { "write": "manager" }
  }
}
```

**Good for:** Simple apps, clear hierarchy
**Limitation:** No action granularity

---

### Option B: Action-Based (AWS IAM style)

Permissions are verbs, not just read/write.

```json
{
  "notes": {
    "actions": {
      "create": "member",
      "edit": "creator",
      "approve": "manager",
      "publish": "admin",
      "delete": "admin"
    }
  }
}
```

**Check:** `CanDo(user, "notes", "approve")` → manager+ only

**Good for:** Workflows with specific actions
**Examples:** CMS publishing, approval flows

---

### Option C: Folder/Space-Based (Confluence style)

Location determines permissions. Moving items changes access.

```json
{
  "spaces": {
    "drafts": {
      "view": "creator",
      "create": "member"
    },
    "pending-review": {
      "view": "manager",
      "move_to_approved": "manager"
    },
    "published": {
      "view": "guest",
      "edit": "admin"
    }
  }
}
```

**Good for:** Content that moves through stages
**Examples:** Document libraries, publishing pipelines

---

### Option D: State Machine + Transitions

Items have states, roles control transitions.

```json
{
  "notes": {
    "states": ["draft", "pending", "approved", "published", "archived"],
    "transitions": {
      "submit_for_review": {
        "from": "draft",
        "to": "pending",
        "role": "member"
      },
      "approve": {
        "from": "pending",
        "to": "approved",
        "role": "manager"
      },
      "publish": {
        "from": "approved",
        "to": "published",
        "role": "admin"
      },
      "archive": {
        "from": ["published", "approved"],
        "to": "archived",
        "role": "admin"
      }
    }
  }
}
```

**Check:** `CanTransition(user, note, "approve")` → is manager AND note is "pending"

**Good for:** Workflow-heavy apps, approval chains
**Examples:** Jira-style, content approval, moderation queues

---

### Option E: Hybrid (Actions + States + Spaces)

Combine all three for maximum flexibility.

```json
{
  "resources": {
    "notes": {
      "space": "team-docs",
      "states": ["draft", "review", "published"],
      "actions": {
        "create": "member",
        "edit": { "role": "member", "states": ["draft"] },
        "submit": { "role": "member", "from": "draft", "to": "review" },
        "approve": { "role": "manager", "from": "review", "to": "published" },
        "unpublish": { "role": "admin", "from": "published", "to": "draft" }
      }
    }
  }
}
```

---

## "Turing Complete" Authorization Systems

For complex requirements, these are the gold standards:

### 1. Casbin (PERM Model) ⭐ LIKELY GO-TO

**What:** Policy engine with pluggable models (RBAC, ABAC, ACL, or custom)
**Language:** Go, Java, Node, Python, Rust, etc.
**Repo:** https://github.com/casbin/casbin
**Like:** CanCanCan for Go (and multi-language)

**Model definition:**
```ini
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act
```

**Policy (CSV or DB):**
```
p, member, notes, create
p, member, notes, edit
p, manager, notes, approve
p, admin, notes, publish
p, admin, notes, delete
```

**Check:** `e.Enforce("alice", "notes", "approve")`

**Superpower:** Custom matchers - regex, ABAC conditions, hierarchy

```ini
# RBAC with hierarchy
[role_definition]
g = _, _

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
```

```
g, alice, member
g, member, guest      # member inherits guest
g, manager, member    # manager inherits member
g, admin, manager     # admin inherits manager
```

---

### 2. Open Policy Agent (OPA)

**What:** General-purpose policy engine, programmable in Rego
**Language:** Rego (declarative, Datalog-inspired)
**Repo:** https://github.com/open-policy-agent/opa

**Policy (Rego):**
```rego
package authz

default allow = false

# Members can create notes
allow {
    input.action == "create"
    input.resource == "notes"
    has_role("member")
}

# Only managers can approve
allow {
    input.action == "approve"
    input.resource == "notes"
    has_role("manager")
}

# Only creator can edit draft
allow {
    input.action == "edit"
    input.resource == "notes"
    input.resource_state == "draft"
    input.user == input.resource_creator
}

# Role hierarchy
has_role(role) {
    role == "guest"
}
has_role(role) {
    role == "member"
    user_role := data.users[input.user].role
    user_role != "guest"
}
has_role(role) {
    role == "manager"
    user_role := data.users[input.user].role
    user_role in {"manager", "admin", "owner"}
}
```

**Superpower:** True programming logic, external data, complex conditions

---

### 3. Cedar (AWS)

**What:** Policy language designed to be fast, analyzable, and expressive
**By:** AWS (used in Amazon Verified Permissions)
**Repo:** https://github.com/cedar-policy/cedar

**Policy:**
```cedar
// Members can create notes
permit (
    principal in Role::"member",
    action == Action::"create",
    resource in ResourceType::"notes"
);

// Managers can approve pending notes
permit (
    principal in Role::"manager",
    action == Action::"approve",
    resource in ResourceType::"notes"
) when {
    resource.state == "pending"
};

// Creator can edit their own drafts
permit (
    principal,
    action == Action::"edit",
    resource in ResourceType::"notes"
) when {
    resource.state == "draft" &&
    resource.creator == principal
};
```

**Superpower:** Analyzable (can prove policies don't conflict), fast evaluation

---

### 4. SpiceDB/Zanzibar (ReBAC)

Already covered above. Best for relationship-heavy models (folders, sharing, teams).

---

## Choosing the Right Model

| Need | Best Fit |
|------|----------|
| Simple role hierarchy | Option A (current) |
| Action-specific permissions | Option B or Casbin |
| Content moving through stages | Option C or D |
| Complex workflows + conditions | OPA or Cedar |
| Sharing, folders, team inheritance | SpiceDB/Zanzibar |
| "I need maximum flexibility" | OPA (truly programmable) |
| "I need provable correctness" | Cedar (analyzable) |

### **Likely Go-To: Casbin** ⭐

Casbin is our preferred choice when we outgrow the simple JSON config because:
- **Familiar** - Same mental model as CanCanCan (Ruby)
- **Go-native** - First-class Go support, fast
- **Flexible** - RBAC, ABAC, ACL, or custom
- **Multi-tenant** - Orgs/domains built-in
- **Mature** - Battle-tested, large community
- **Storage** - Firestore adapter available

See `services/game-service/casbin_example/` for working examples.

---

## Implementation Path

1. **Now:** Simple JSON config (Option A) ✓
2. **Next:** Casbin when we need action-based permissions ⭐
3. **Later:** Add state machine transitions via Casbin policies
4. **Scale:** Casbin handles it (used by Intel, IBM, VMware)

Migration path is clean - our `org.json` format can be loaded into Casbin policies.

---

## Sources

- [Google Zanzibar Paper](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/)
- [SpiceDB GitHub](https://github.com/authzed/spicedb)
- [SpiceDB Schema Docs](https://authzed.com/docs/spicedb/concepts/schema)
- [Notion Permissions Guide](https://www.notion.com/help/sharing-and-permissions)
- [Notion Roles](https://www.notion.com/help/add-members-admins-guests-and-groups)
- [GitHub Repository Roles](https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories/managing-repository-roles/repository-roles-for-an-organization)
- [GitHub Access Permissions](https://docs.github.com/en/get-started/learning-about-github/access-permissions-on-github)
- [Casbin](https://casbin.org/)
- [Open Policy Agent](https://www.openpolicyagent.org/)
- [Cedar Policy Language](https://www.cedarpolicy.com/)
- [AWS Verified Permissions](https://aws.amazon.com/verified-permissions/)
- [Directus Workflows](https://directus.io/docs/tutorials/workflows/build-content-approval-workflows-with-custom-permissions)
- [Confluence Permissions](https://support.atlassian.com/confluence-cloud/docs/what-are-space-permissions/)
