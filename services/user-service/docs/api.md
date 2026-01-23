# User Service API

Thin wrapper around Firebase Auth. Handles user profile storage in Firestore.

## Auth

All endpoints require Firebase ID token:

```
Authorization: Bearer <firebase-id-token>
```

---

## Endpoints

### POST /users/sync

Called after first login. Creates user doc if not exists, returns profile.

**Request:** None (user ID from token)

**Response:**
```json
{
  "id": "firebase-uid",
  "email": "user@example.com",
  "createdAt": "2024-01-15T10:30:00Z",
  "updatedAt": "2024-01-15T10:30:00Z",
  "metadata": {}
}
```

**Status codes:**
- `200` - User exists, returned
- `201` - User created, returned
- `401` - Invalid/missing token

---

### GET /users/me

Get current user profile.

**Request:** None

**Response:**
```json
{
  "id": "firebase-uid",
  "email": "user@example.com",
  "createdAt": "2024-01-15T10:30:00Z",
  "updatedAt": "2024-01-15T10:30:00Z",
  "metadata": {
    "onboardingComplete": true
  }
}
```

**Status codes:**
- `200` - Success
- `401` - Invalid/missing token
- `404` - User not found (call /sync first)

---

### PATCH /users/me

Update user metadata.

**Request:**
```json
{
  "metadata": {
    "onboardingComplete": true,
    "preferences": {
      "theme": "dark"
    }
  }
}
```

**Response:**
```json
{
  "id": "firebase-uid",
  "email": "user@example.com",
  "createdAt": "2024-01-15T10:30:00Z",
  "updatedAt": "2024-01-15T12:00:00Z",
  "metadata": {
    "onboardingComplete": true,
    "preferences": {
      "theme": "dark"
    }
  }
}
```

**Status codes:**
- `200` - Updated
- `401` - Invalid/missing token
- `404` - User not found

---

### DELETE /users/me

Delete user from Firestore and Firebase Auth.

**Request:** None

**Response:**
```json
{
  "deleted": true
}
```

**Status codes:**
- `200` - Deleted
- `401` - Invalid/missing token
- `404` - User not found

---

## Client Flow (Magic Link)

```
1. App: Firebase.auth().sendSignInLinkToEmail(email)
2. Firebase sends magic link email
3. User clicks link → opens app via deep link
4. App: Firebase.auth().signInWithEmailLink(email, link)
5. App: token = Firebase.auth().currentUser.getIdToken()
6. App: POST /users/sync with Authorization header
7. App: use /users/me for subsequent profile operations
```

---

## Error Response Format

All errors return:

```json
{
  "error": {
    "code": "not_found",
    "message": "User not found"
  }
}
```

Common codes:
- `unauthorized` - Invalid/missing token
- `not_found` - Resource not found
- `bad_request` - Invalid request body
- `internal` - Server error
