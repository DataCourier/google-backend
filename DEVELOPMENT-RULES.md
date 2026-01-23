# Development Rules

## **RULE #1: NEVER Deploy Without Testing Locally First** 🚫

### The Problem

```
Edit code → Deploy to Cloud Run (2 min) → Test → It's broken → Fix → Deploy again (2 min) → ...
                         ↑                                              ↑
                   WASTED 2 MINUTES!                              WASTED ANOTHER 2 MINUTES!
```

**Result:** 5-10 minutes wasted on a simple bug that takes 2 seconds to fix locally.

### The Rule

```
✅ CORRECT WORKFLOW:
1. Edit code
2. Test locally (2 seconds)
3. Fix any issues locally
4. ONLY deploy when it works locally
5. Test on Cloud Run

❌ WRONG WORKFLOW:
1. Edit code
2. Deploy immediately (2 minutes)
3. Find it's broken
4. Go back to step 1
```

### How to Test Locally

```bash
# Terminal 1: Run server
cd services/game-service
export PATH=/home/michal/go/bin:$PATH
export GCP_PROJECT=michal-playground-2026
go run .

# Terminal 2: Test
curl http://localhost:8080/
curl "http://localhost:8080/games/test?token=abc"
```

### Checklist Before Deploy

- [ ] Tested homepage locally
- [ ] Tested all changed endpoints locally
- [ ] Checked logs for errors
- [ ] Verified template renders correctly
- [ ] Made at least one successful test request

**ONLY THEN:** `make deploy-game`

---

## RULE #2: Use Local Dev for Iteration

### Fast Feedback Loop

```
Edit → Test locally (2s) → Fix → Test (2s) → Repeat until working → Deploy once
```

### Slow Feedback Loop (DON'T DO THIS)

```
Edit → Deploy (2min) → Test → Fix → Deploy (2min) → Test → ...
```

---

## RULE #3: Commit Often (But Don't Push Every Time)

### Good

```bash
git add -A
git commit -m "WIP: testing template fix"  # Local commit
# Test more
git commit -m "Fix template rendering"     # Local commit
# Test again, it works!
git push                                    # Push when done
```

### Bad

```bash
git commit -m "try fix 1" && git push
# Deploy, test, broken
git commit -m "try fix 2" && git push
# Deploy, test, broken
git commit -m "try fix 3" && git push
# Result: messy git history, wasted time
```

---

## RULE #4: Check Logs Before Assuming

### Good

```bash
# Make request
curl http://localhost:8080/games/test

# Check logs immediately
# Look for → handler entry, ✓ success, ✗ errors
# See which template rendered
# See any error messages
```

### Bad

```bash
# Make request
curl http://localhost:8080/games/test
# It shows wrong page

# Immediately start changing code without checking logs
# Waste time guessing what's wrong
```

---

## RULE #5: Document Decisions

When you spend >10 minutes on something (like template inheritance), document:
- What you tried
- Why it didn't work
- What the solution was
- Better alternatives

**Example:** We created `TEMPLATE-OPTIONS.md` after fighting with `html/template`.

---

## RULE #6: Ask "Should I use a library?" Early

### Signs you're reinventing the wheel:

- Spending >30 minutes on something basic
- Writing lots of boilerplate
- Fighting with the stdlib

### When to reach for a library:

- Templates: Use `templ` or `gomponents`
- Routing: Use `chi` (we already did this!)
- Validation: Use `validator`
- Database: Use an ORM if SQL gets messy

---

## RULE #7: Harry Potter + Beer = Better Code

But also:
- Don't deploy drunk
- Test locally first (see Rule #1)
- Document your brilliant ideas before you forget them

---

## Summary

**The One Rule to Rule Them All:**

> Test locally. Deploy once. Save time.

**Time Savings:**

Without local dev: 10-20 minutes per feature (multiple deploys)
With local dev: 2-5 minutes per feature (one deploy)

**4x faster development!**
