package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
)

type Doc struct {
	ID           string
	UserID       string
	DedupeValue  string
	ProtectValue string
	CreatedAt    time.Time
}

type DedupeResult struct {
	TotalDocs      int
	DuplicateGroups int
	ToDelete       []Doc
	Protected      int
}

// findDuplicates groups docs by (userID, dedupeField) and returns which to delete.
// Records with a non-empty protectField are never deleted.
// Among unprotected duplicates, the oldest (by created_at) is kept.
func findDuplicates(docs []Doc) DedupeResult {
	type groupKey struct{ userID, dedupeValue string }
	groups := map[groupKey][]Doc{}

	for _, d := range docs {
		k := groupKey{d.UserID, d.DedupeValue}
		groups[k] = append(groups[k], d)
	}

	var result DedupeResult
	result.TotalDocs = len(docs)

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		result.DuplicateGroups++

		var protected, unprotected []Doc
		for _, d := range group {
			if d.ProtectValue != "" {
				protected = append(protected, d)
			} else {
				unprotected = append(unprotected, d)
			}
		}

		result.Protected += len(protected)

		// If there are protected records, they're all kept.
		// Among unprotected, keep the oldest.
		if len(unprotected) > 0 {
			sort.Slice(unprotected, func(i, j int) bool {
				return unprotected[i].CreatedAt.Before(unprotected[j].CreatedAt)
			})
			// Keep the first unprotected only if no protected records exist
			start := 0
			if len(protected) == 0 {
				start = 1
			}
			result.ToDelete = append(result.ToDelete, unprotected[start:]...)
		}
	}

	return result
}

func main() {
	project := flag.String("project", "", "GCP project ID (required)")
	collection := flag.String("collection", "personal-videos", "Firestore collection")
	dedupeOn := flag.String("dedupe-on", "video_id", "Field to deduplicate on")
	protectField := flag.String("protect-field", "notes", "Never delete records with this field non-empty")
	dryRun := flag.Bool("dry-run", true, "Print what would be deleted without deleting")
	flag.Parse()

	if *project == "" {
		log.Fatal("--project is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	// Fetch all documents
	fmt.Printf("Fetching all docs from %s...\n", *collection)
	iter := client.Collection(*collection).Documents(ctx)
	defer iter.Stop()

	var docs []Doc
	for {
		snap, err := iter.Next()
		if err != nil {
			break
		}
		data := snap.Data()

		d := Doc{ID: snap.Ref.ID}
		if v, ok := data["user_id"].(string); ok {
			d.UserID = v
		}
		if v, ok := data[*dedupeOn].(string); ok {
			d.DedupeValue = v
		}
		if v, ok := data[*protectField].(string); ok {
			d.ProtectValue = v
		}
		if v, ok := data["created_at"].(time.Time); ok {
			d.CreatedAt = v
		}
		docs = append(docs, d)
	}

	result := findDuplicates(docs)

	fmt.Printf("\n=== Dedupe Summary ===\n")
	fmt.Printf("Total documents:   %d\n", result.TotalDocs)
	fmt.Printf("Duplicate groups:  %d\n", result.DuplicateGroups)
	fmt.Printf("Protected (notes): %d\n", result.Protected)
	fmt.Printf("To delete:         %d\n", len(result.ToDelete))

	if len(result.ToDelete) == 0 {
		fmt.Println("\nNothing to delete.")
		return
	}

	fmt.Println("\nDocuments to delete:")
	for _, d := range result.ToDelete {
		fmt.Printf("  - %s (user=%s, %s=%s, created=%s)\n",
			d.ID, d.UserID, *dedupeOn, d.DedupeValue, d.CreatedAt.Format(time.RFC3339))
	}

	if *dryRun {
		fmt.Println("\n[DRY RUN] No documents were deleted. Pass --dry-run=false to delete.")
		return
	}

	fmt.Println("\nDeleting...")
	deleted := 0
	for _, d := range result.ToDelete {
		_, err := client.Collection(*collection).Doc(d.ID).Delete(ctx)
		if err != nil {
			fmt.Printf("  ERROR deleting %s: %v\n", d.ID, err)
		} else {
			deleted++
		}
	}
	fmt.Printf("Deleted %d/%d documents.\n", deleted, len(result.ToDelete))
}
