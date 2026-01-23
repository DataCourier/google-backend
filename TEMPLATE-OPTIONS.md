# Template Options for Go

We're currently fighting with `html/template` inheritance. Here are better options:

---

## Current Problem

**Go's `html/template` with inheritance is painful:**
- `{{template "layout.html" .}}` needs specific parsing order
- `ParseGlob` doesn't guarantee order
- Easy to mess up, hard to debug
- We wasted 30 minutes on this

---

## Option 1: Inline Templates (Current Quick Fix) ✅

**Pros:**
- ✅ Works immediately
- ✅ No inheritance complexity
- ✅ Zero dependencies
- ✅ Easy to understand

**Cons:**
- ❌ Duplicate header/footer HTML
- ❌ Not DRY

**When to use:** Quick projects, simple sites, prototypes

**Example:** `views/game-inline.html` (what we just did)

---

## Option 2: templ (Recommended for Production) 🌟

**Type-safe templates that compile to Go code.**

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

**Example:**

```go
// views/layout.templ
package views

templ Layout(title string) {
    <!DOCTYPE html>
    <html>
    <head><title>{ title }</title></head>
    <body>
        <header><h1>Tic-Tac-Toe</h1></header>
        <main>
            { children... }
        </main>
    </body>
    </html>
}

// views/game.templ
package views

import "yourapp/models"

templ GamePage(game *models.Game, player string) {
    @Layout("Game " + game.ID) {
        <div class="status">You are { player }</div>
        <div class="board">
            for i, cell := range game.Board {
                <div class="cell" onclick={ makeMove(i) }>
                    { cell }
                </div>
            }
        </div>
    }
}
```

**Usage in handler:**

```go
func viewGameHandler(w http.ResponseWriter, r *http.Request) {
    // ... get game data ...

    // Render with type safety!
    views.GamePage(game, player).Render(context.Background(), w)
}
```

**Pros:**
- ✅ **Type-safe** - Compile-time errors if you mess up!
- ✅ **Fast** - Compiles to Go code
- ✅ **IDE support** - Autocomplete, refactoring
- ✅ **Component-based** - Like React
- ✅ **No runtime template parsing**

**Cons:**
- Adds build step (`templ generate`)
- New syntax to learn (but it's simple)

**When to use:** Production apps, larger projects

---

## Option 3: gomponents

**Write HTML in pure Go (like React components).**

```bash
go get github.com/maragudk/gomponents
```

**Example:**

```go
import (
    . "github.com/maragudk/gomponents"
    . "github.com/maragudk/gomponents/html"
)

func Layout(title string, content Node) Node {
    return HTML(
        Head(Title(Text(title))),
        Body(
            Header(H1(Text("Tic-Tac-Toe"))),
            Main(content),
        ),
    )
}

func GamePage(game *models.Game, player string) Node {
    cells := make([]Node, 9)
    for i, cell := range game.Board {
        cells[i] = Div(
            Class("cell"),
            Onclick(makeMove(i)),
            Text(cell),
        )
    }

    return Layout("Game "+game.ID,
        Div(
            Div(Class("status"), Textf("You are %s", player)),
            Div(Class("board"), cells...),
        ),
    )
}
```

**Usage:**

```go
func viewGameHandler(w http.ResponseWriter, r *http.Request) {
    page := GamePage(game, player)
    page.Render(w)
}
```

**Pros:**
- ✅ **100% Go** - No new syntax
- ✅ **Type-safe** - Compiler checks everything
- ✅ **Refactorable** - IDE can rename/move
- ✅ **Composable** - Functions return components

**Cons:**
- Verbose for simple HTML
- No syntax highlighting for HTML structure

**When to use:** If you prefer Go over template syntax

---

## Option 4: Just Use React/HTMX + API

**Separate frontend entirely.**

```go
// Go just serves JSON API
func viewGameHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(game)
}
```

Frontend: React, Vue, HTMX, or even vanilla JS.

**Pros:**
- ✅ Complete separation of concerns
- ✅ Use best frontend tools
- ✅ Go does what it's good at (APIs)

**Cons:**
- More complex deployment
- Need frontend build step

**When to use:** Larger apps, mobile apps with web dashboard

---

## Recommendation

### For This Project (Tic-Tac-Toe)

**Short term:** Inline templates (what we just did)
- Gets it working NOW
- Simple, no dependencies

**Long term:** Switch to `templ`
- Type-safe
- Better DX
- Production-ready

### Migration Path

1. **Now:** Use inline templates (done!)
2. **Next session:** Try `templ` for one page
3. **If you like it:** Migrate rest of templates
4. **If not:** Stick with inline or try gomponents

---

## How to Add templ

```bash
# Install
go install github.com/a-h/templ/cmd/templ@latest

# Create views/game.templ
# Write template (see example above)

# Generate Go code
templ generate

# Import and use in handler
import "yourapp/views"
views.GamePage(game, player).Render(ctx, w)

# Add to Makefile
templ-watch:
    templ generate --watch
```

---

## Summary

| Option | Complexity | Type Safety | Performance | DRY |
|--------|-----------|-------------|-------------|-----|
| html/template | Medium | ❌ | Good | ✅ (if you can make it work!) |
| **Inline** | Low | ❌ | Good | ❌ |
| **templ** | Medium | ✅ | Excellent | ✅ |
| **gomponents** | Low | ✅ | Excellent | ✅ |
| API + React | High | ✅ | Good | ✅ |

**My vote:** `templ` for production, inline for quick wins.

---

## Resources

- [templ](https://templ.guide/)
- [gomponents](https://www.gomponents.com/)
- [HTMX](https://htmx.org/) (if you want HTML-over-the-wire)
