# Go Performance Notes

## Compilation Speed (Still Lightning Fast!)

### Local Development Builds

```bash
# First build (no cache)
$ time go build .
real    0m9.009s   # Includes linking Firestore SDK, chi, etc.

# Subsequent builds (with cache) - only rebuilds changed files
$ time go build .
real    0m0.002s   # 2 MILLISECONDS! ⚡
```

**Go's build cache is magical:**
- Only recompiles changed files
- Incremental linking
- Smart dependency tracking
- 99.98% faster on subsequent builds

### Why Cloud Run Takes 2 Minutes

It's **not Go** - it's infrastructure overhead:

```
Cloud Run Deploy (~120 seconds):
├─ Upload source:              ~5-10s
├─ Cloud Build VM spin-up:     ~20-30s   ← Infrastructure
├─ Buildpack detection:        ~5s       ← Container stuff
├─ Download deps (no cache!):  ~20-30s   ← Network I/O
├─ Go compilation:             ~5-10s    ← This is Go
├─ Container build:            ~20s      ← Docker/Buildpack
├─ Push to registry:           ~10-15s   ← Network I/O
├─ Cloud Run deployment:       ~10-20s   ← Infrastructure
└─ Health checks:              ~5-10s    ← Infrastructure

Total: ~120s (only ~10s is Go!)
```

### Local Dev (With Cache)

```
Edit → Build (0.002s) → Run (instant) → Test
Total: ~2 seconds

60x faster than Cloud Run!
```

---

## Go vs Other Languages

### Compilation Speed (Medium Project)

| Language | Clean Build | Incremental Build |
|----------|-------------|-------------------|
| Go       | 9s          | **0.002s** ⚡     |
| Rust     | 60-120s     | 10-30s            |
| Java     | 30-60s      | 5-15s             |
| C++      | 120-300s    | 20-60s            |

### Why Go is Still Fast

1. **Simple imports** - No header files, no circular deps
2. **Fast linker** - Designed for speed from day 1
3. **Build cache** - Aggressive caching of unchanged code
4. **Parallel compilation** - Uses all CPU cores
5. **Single pass** - No need for multiple compilation passes

---

## Go Build Cache Explained

### Where is the cache?

```bash
# View cache location
go env GOCACHE
# Usually: ~/.cache/go-build

# Cache size
du -sh $(go env GOCACHE)
# Example: 2.5GB

# Clear cache (if needed)
go clean -cache
```

### What gets cached?

- Compiled packages
- Object files
- Test results
- Dependency analysis

### When cache is invalidated?

- Source file changes
- Dependency changes
- Compiler flags change
- Go version change

---

## Cloud Run Optimization Tips

### 1. Use Docker with Multi-Stage Builds

Instead of buildpacks, use a Dockerfile with caching:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /build

# Copy go.mod first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source (invalidates cache only when code changes)
COPY . .
RUN go build -o game-service .

FROM alpine:latest
COPY --from=builder /build/game-service .
CMD ["./game-service"]
```

**Result:** ~60-90s instead of 120s (deps cached in Docker layers)

### 2. Use Cloud Build Caching

```yaml
# cloudbuild.yaml
options:
  machineType: 'N1_HIGHCPU_8'  # Faster machine
  substitution_option: 'ALLOW_LOOSE'
  logging: CLOUD_LOGGING_ONLY

steps:
  - name: 'gcr.io/cloud-builders/docker'
    args: ['build', '-t', 'gcr.io/$PROJECT_ID/game-service', '--cache-from', 'gcr.io/$PROJECT_ID/game-service:latest', '.']
```

### 3. Keep Dependencies Minimal

```bash
# Check dependency tree
go mod graph | wc -l

# Remove unused deps
go mod tidy

# Vendor for faster builds (optional)
go mod vendor
```

---

## The Real Problem: Not Go, But Feedback Loop

### Problem

```
Edit code → Deploy to Cloud Run (2 min) → Test → Repeat
                         ↑
                    Slow feedback loop
```

### Solution: Local Development

```
Edit code → Local build (0.002s) → Test → Repeat
                         ↑
                    Instant feedback!
```

**This is why we set up local dev!**

---

## Go Performance Over Time

### Go 1.0 (2012)
- Fast compilation
- Good runtime performance

### Go 1.11 (2018)
- Modules system
- Build cache introduced ← Game changer!

### Go 1.17 (2021)
- Register-based calling convention
- 5-10% faster compilation

### Go 1.20+ (2023+)
- PGO (Profile-Guided Optimization)
- Faster linker
- Better inlining

**Go has gotten FASTER over time, not slower!**

---

## Benchmarks: Local vs Cloud

### Local Development (This Project)

```bash
# Edit main.go
time go build .        # 0.002s
./game-service &       # instant
curl localhost:8080    # test immediately

Total: ~2 seconds
```

### Cloud Run Deployment (This Project)

```bash
# Edit main.go
gcloud run deploy ...  # 120s (2 minutes)
curl https://...       # test after deploy

Total: ~120 seconds
```

**Ratio: 60x slower on Cloud Run**

---

## Summary

✅ **Go compilation is still blazingly fast**
- 2ms with cache
- 9s cold build (with huge dependencies)
- Still faster than Rust, Java, C++

❌ **Cloud Run is slow due to infrastructure**
- Not Go's fault
- Network I/O, VM spin-up, container builds
- No build cache between deploys

💡 **Solution: Local Development**
- Iterate locally (2s)
- Deploy when ready (2min)
- Best of both worlds!

---

## Further Reading

- [Go Build Cache](https://golang.org/doc/go1.11#cache)
- [Go Compilation Performance](https://go.dev/blog/rebuild)
- [Faster Docker Builds](https://docs.docker.com/build/cache/)
