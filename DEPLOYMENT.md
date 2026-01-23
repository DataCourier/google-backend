# Deployment Guide

Complete step-by-step guide for deploying services to Google Cloud Run with Firestore.

---

## Prerequisites

1. **Google Cloud Project**
   - Create project at https://console.cloud.google.com
   - Note your project ID (e.g., `michal-playground-2026`)

2. **gcloud CLI installed and configured**
   ```bash
   gcloud --version
   gcloud auth login
   gcloud config set project YOUR_PROJECT_ID
   ```

3. **Firebase/Firestore Setup**
   - Visit https://console.firebase.google.com
   - Add Firebase to your Google Cloud project (or create new)

---

## Initial Setup (One-Time)

### 1. Enable Required APIs

```bash
# Enable Cloud Run API
gcloud services enable run.googleapis.com

# Enable Cloud Build API (for building containers)
gcloud services enable cloudbuild.googleapis.com

# Enable Firestore API
gcloud services enable firestore.googleapis.com

# Enable Artifact Registry (for storing container images)
gcloud services enable artifactregistry.googleapis.com
```

### 2. Create Firestore Database

**Option A: Via gcloud CLI** (Recommended)
```bash
# Create Firestore native database in your preferred region
gcloud firestore databases create \
  --location=us-central1 \
  --type=firestore-native
```

**Option B: Via Firebase Console**
1. Visit https://console.firebase.google.com/project/YOUR_PROJECT_ID/firestore
2. Click "Create Database"
3. Choose "Production mode" or "Test mode"
4. Select location (e.g., us-central1)

**Verify database creation:**
```bash
gcloud firestore databases list
```

You should see:
```
NAME      LOCATION       TYPE              CREATE_TIME
(default) us-central1    FIRESTORE_NATIVE  2026-01-23T21:16:36
```

---

## Deployment Process

### Step 1: Configure Your Project

Edit `config.yaml`:
```yaml
project:
  name: "my-app"
  firebase_project_id: "YOUR_PROJECT_ID"  # ← Change this!
  gcp_region: "us-central1"
  module_path: "github.com/yourusername/my-app-backend"
```

### Step 2: Run Setup

This syncs shared code to all services:
```bash
./setup.sh
```

You should see:
```
🚀 Backend Setup Script
📋 Configuration:
   Project Name: my-app
   Module Path:  github.com/yourusername/my-app-backend
   Firebase:     YOUR_PROJECT_ID
...
✅ Setup complete!
```

### Step 3: Deploy Services

**Deploy all services:**
```bash
make deploy
```

**Or deploy individually:**
```bash
# User service
make deploy-user

# Game service
make deploy-game

# Org service (when implemented)
make deploy-org
```

**Manual deployment (if needed):**
```bash
cd services/game-service
gcloud run deploy game-service \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars GCP_PROJECT=YOUR_PROJECT_ID
```

### Step 4: Test Deployment

After deployment, you'll see:
```
Service URL: https://game-service-XXXXXXXXXX.us-central1.run.app
```

Test it:
```bash
curl https://game-service-XXXXXXXXXX.us-central1.run.app/
```

---

## How Cloud Run Deployment Works

### Buildpack Auto-Detection

Cloud Run uses Google Cloud Buildpacks to **automatically detect Go** and build your app:

1. **No Dockerfile needed** - Buildpacks detect `go.mod`
2. **Auto-installs dependencies** - Runs `go mod download`
3. **Builds binary** - Runs `go build`
4. **Creates container** - Packages everything
5. **Deploys to Cloud Run** - Serves on port 8080 (via `PORT` env var)

### What Gets Deployed

From `services/game-service/`:
```
main.go              → Compiled to binary
models/              → Compiled into binary
views/*.html         → Included in container
go.mod               → Used for dependencies
auth/                → Compiled into binary (synced from /shared)
middleware/          → Compiled into binary (synced from /shared)
response/            → Compiled into binary (synced from /shared)
```

### Environment Variables

Cloud Run automatically provides:
- `PORT` - Port to listen on (usually 8080)
- `GOOGLE_APPLICATION_CREDENTIALS` - Auth for Firestore (automatic)

You can set custom vars:
```bash
--set-env-vars GCP_PROJECT=your-project-id,ENV=production
```

---

## Common Issues & Solutions

### Issue 1: "Firestore API not enabled"

**Error:**
```
Cloud Firestore API has not been used in project...
```

**Solution:**
```bash
gcloud services enable firestore.googleapis.com
```

---

### Issue 2: "Database does not exist"

**Error:**
```
The database (default) does not exist for project...
```

**Solution:**
```bash
gcloud firestore databases create --location=us-central1 --type=firestore-native
```

---

### Issue 3: "Permission denied"

**Error:**
```
Permission denied to access Firestore
```

**Solution:**
Cloud Run service account needs Firestore permissions:
```bash
# Get Cloud Run service account
PROJECT_NUMBER=$(gcloud projects describe YOUR_PROJECT_ID --format="value(projectNumber)")
SERVICE_ACCOUNT="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

# Grant Firestore access
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/datastore.user"
```

---

### Issue 4: Build fails with "go.sum not found"

**Error:**
```
COPY failed: no source files were specified
```

**Solution:**
Don't use a Dockerfile! Remove it and let buildpacks auto-detect Go:
```bash
rm Dockerfile
gcloud run deploy ... --source .
```

---

## Deployment Checklist

Before deploying a new service:

- [ ] Service has `go.mod` file
- [ ] Service has `main.go` with `main()` function
- [ ] Server listens on `PORT` env var (defaults to 8080)
- [ ] Run `./setup.sh` to sync shared code
- [ ] Test locally (optional): `go run main.go`
- [ ] Deploy with `make deploy-SERVICENAME`
- [ ] Test deployed URL

---

## Viewing Logs

**Real-time logs:**
```bash
gcloud run services logs tail game-service --region=us-central1
```

**Recent errors:**
```bash
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.service_name=game-service AND severity>=ERROR" \
  --limit=20 \
  --format="table(timestamp,textPayload)"
```

---

## Cost Optimization

### Free Tier Limits

Cloud Run free tier (per month):
- 2 million requests
- 360,000 GB-seconds
- 180,000 vCPU-seconds

Firestore free tier (per day):
- 50,000 document reads
- 20,000 document writes
- 20,000 document deletes
- 1 GB storage

### Tips to Stay Free

1. **Scale to zero** - Cloud Run scales to 0 when idle (default)
2. **Limit memory** - Use 256 MB or 512 MB instances
3. **Set max instances** - Prevent accidental scaling
   ```bash
   gcloud run deploy game-service \
     --max-instances=10 \
     --memory=512Mi
   ```
4. **Use Firestore wisely** - Cache reads, batch writes

---

## CI/CD (Optional)

### GitHub Actions Example

Create `.github/workflows/deploy.yml`:
```yaml
name: Deploy to Cloud Run

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - uses: google-github-actions/auth@v1
        with:
          credentials_json: ${{ secrets.GCP_SA_KEY }}

      - uses: google-github-actions/setup-gcloud@v1

      - name: Deploy game-service
        run: |
          ./setup.sh
          make deploy-game
```

---

## Rollback

If deployment fails or has issues:

```bash
# List revisions
gcloud run revisions list --service=game-service --region=us-central1

# Rollback to previous revision
gcloud run services update-traffic game-service \
  --to-revisions=game-service-00001=100 \
  --region=us-central1
```

---

## Next Steps

- [ ] Set up custom domain (optional)
- [ ] Add Firestore security rules
- [ ] Set up monitoring/alerts
- [ ] Configure Cloud Scheduler for backups (optional)
- [ ] Set up staging environment

---

## Summary

**To deploy a new app:**
```bash
cp -r backend my-new-app
cd my-new-app
nano config.yaml          # Edit project ID
./setup.sh                # Sync shared code
make deploy               # Deploy all services
```

**That's it!** Cloud Run handles the rest.
