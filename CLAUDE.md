# Backend Development Rules

## Critical: No Bucket-Specific Code

**NEVER write code that checks for specific bucket names** like `if bucketName == "blocks"` or `if bucketName == "notes"`.

The bucket system must remain generic. Any feature that works for one bucket type must work for ALL bucket types without special-casing.

### Bad (anti-pattern):
```go
if bucketName == "blocks" {
    // special handling for blocks
}
```

### Good (generic):
```go
// Works for any bucket - inspects data structure, not bucket name
if fieldWithLocations := findFieldWithLocations(data); fieldWithLocations != nil {
    // handle any record that has a locations field
}
```

## File Upload Handler

The upload handler at `services/game-service/buckets/router.go` must:
- Set `file_url` on upload (current behavior)
- Also set `locations.server` in any field that has a `locations` map
- Do this generically by inspecting the record structure, not by bucket name
