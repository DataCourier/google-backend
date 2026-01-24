// Example: Casbin with Org Notes
// Run: go run example.go
//
// Install: go get github.com/casbin/casbin/v2

package main

import (
	"fmt"
	"log"

	"github.com/casbin/casbin/v2"
)

func main() {
	// Load model and policy
	e, err := casbin.NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Org Notes Permission Checks ===\n")

	// Test cases
	checks := []struct {
		user     string
		org      string
		resource string
		action   string
	}{
		// Alice (member) checks
		{"alice", "acme-corp", "notes", "create"},       // ✓ member can create
		{"alice", "acme-corp", "notes", "approve"},      // ✗ member can't approve
		{"alice", "acme-corp", "notes", "view_published"}, // ✓ member inherits guest

		// Bob (manager) checks
		{"bob", "acme-corp", "notes", "create"},         // ✓ manager inherits member
		{"bob", "acme-corp", "notes", "approve"},        // ✓ manager can approve
		{"bob", "acme-corp", "notes", "publish"},        // ✗ manager can't publish
		{"bob", "acme-corp", "projects", "create"},      // ✓ manager can create projects

		// Charlie (admin) checks
		{"charlie", "acme-corp", "notes", "publish"},    // ✓ admin can publish
		{"charlie", "acme-corp", "notes", "delete"},     // ✓ admin can delete
		{"charlie", "acme-corp", "announcements", "create"}, // ✓ admin can announce

		// Eve (guest) checks
		{"eve", "acme-corp", "notes", "create"},         // ✗ guest can't create
		{"eve", "acme-corp", "notes", "view_published"}, // ✓ guest can view published

		// Cross-org check (alice is member of acme, not other-corp)
		{"alice", "other-corp", "notes", "create"},      // ✗ not a member there
	}

	for _, c := range checks {
		allowed, _ := e.Enforce(c.user, c.org, c.resource, c.action)
		status := "✗"
		if allowed {
			status = "✓"
		}
		fmt.Printf("%s %s.%s.%s = %s\n", status, c.user, c.resource, c.action, c.org)
	}

	fmt.Println("\n=== Dynamic Role Assignment ===\n")

	// Add new user to org at runtime
	e.AddGroupingPolicy("frank", "member", "acme-corp")

	allowed, _ := e.Enforce("frank", "acme-corp", "notes", "create")
	fmt.Printf("frank (new member) can create notes: %v\n", allowed)

	// Promote bob to admin
	e.RemoveGroupingPolicy("bob", "manager", "acme-corp")
	e.AddGroupingPolicy("bob", "admin", "acme-corp")

	allowed, _ = e.Enforce("bob", "acme-corp", "notes", "publish")
	fmt.Printf("bob (promoted to admin) can publish: %v\n", allowed)
}
