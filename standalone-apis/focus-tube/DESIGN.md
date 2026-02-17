# FocusTube - Personal Focused YouTube

A single-user, distraction-free YouTube subscription tracker. You own all your data in personal buckets.

## Concept

Strip YouTube down to what matters: your subscriptions, their latest videos, and whether you liked them. No algorithm, no recommendations, no shorts, no comments. Just a chronological feed from channels you chose.

## Architecture

```
standalone-apis/focus-tube/
├── main.go              # Go server: bucket API + static file serving
├── buckets/             # Copied from game-service/buckets (personal bucket CRUD)
├── auth/                # Copied from game-service/auth (local dev tokens)
├── frontend/            # React SPA (Vite + Tailwind)
│   ├── src/
│   │   ├── App.jsx
│   │   ├── api.js       # Bucket & RSS API client
│   │   └── components/
│   │       ├── Feed.jsx
│   │       ├── VideoCard.jsx
│   │       ├── ChannelList.jsx
│   │       └── AddChannel.jsx
│   ├── index.html
│   ├── package.json
│   ├── vite.config.js
│   └── tailwind.config.js
├── go.mod
└── DESIGN.md
```

**Backend**: Copy the personal bucket system from game-service. Single-user, local-first. Auth via `Authorization: local:michal` header (hardcoded in the frontend for now).

**Frontend**: React SPA with Vite and Tailwind CSS. During dev, Vite dev server proxies API calls to the Go backend. For production, `vite build` outputs to `frontend/dist/` which Go serves as static files.

## Data Model (Buckets)

### Bucket: `channels`

Each record = one YouTube channel subscription.

```json
{
  "id": "UC...",
  "user_id": "michal",
  "channel_name": "3Blue1Brown",
  "channel_url": "https://www.youtube.com/@3blue1brown",
  "rss_url": "https://www.youtube.com/feeds/videos.xml?channel_id=UC...",
  "thumbnail_url": "https://yt3.ggpht.com/...",
  "added_at": 1708200000000,
  "created_at": 1708200000000,
  "updated_at": 1708200000000
}
```

**Adding a channel**: User pastes a YouTube channel URL (e.g. `youtube.com/@3blue1brown`). JS in the browser:
1. Fetches the channel page (or uses YouTube's oEmbed/noembed API) to resolve the channel ID
2. Constructs the RSS URL: `https://www.youtube.com/feeds/videos.xml?channel_id=UCXXXX`
3. Saves the channel record to the `channels` bucket

### Bucket: `videos`

Each record is *yours* — not a YouTube database entry, but a personal item that happens to reference a video. It starts as an inbox item from RSS sync, and grows into a note as you watch, react, and write.

```json
{
  "id": "dQw4w9WgXcQ",
  "user_id": "michal",
  "channel_id": "UC...",
  "channel_name": "3Blue1Brown",
  "title": "But what is a neural network?",
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "thumbnail_url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg",
  "published_at": 1708200000000,
  "watched": false,
  "favorited": false,
  "hidden": false,
  "notes": "",
  "created_at": 1708200000000,
  "updated_at": 1708200000000
}
```

**Lifecycle**: RSS sync creates the record (inbox). You watch it, maybe favorite it, maybe jot down notes. A video with `favorited: true` or non-empty `notes` is a "note" — something you kept. The app can show these as a personal library/notebook view filtered from the same bucket.

## How RSS Feeds Work

YouTube exposes RSS for every channel at:
```
https://www.youtube.com/feeds/videos.xml?channel_id=CHANNEL_ID
```

This returns the ~15 most recent videos as Atom XML. No API key needed.

**Problem**: Browser JS can't fetch cross-origin YouTube RSS directly (CORS).

**Solution**: Add a server-side `/api/fetch-rss?channel_id=UCXXX` endpoint in the Go backend that proxies the RSS fetch, parses the XML, and returns JSON. This is the one non-bucket endpoint.

Similarly, resolving a channel URL to a channel ID needs server-side help: `/api/resolve-channel?url=https://youtube.com/@3blue1brown` — fetches the page and extracts the channel ID from meta tags.

## Endpoints

### From bucket system (copied from game-service):
- `GET /buckets/mine/{bucket}` — list records
- `POST /buckets/mine/{bucket}` — create record
- `PUT /buckets/mine/{bucket}/{id}` — update record
- `DELETE /buckets/mine/{bucket}/{id}` — delete record

### Custom (focus-tube specific):
- `GET /api/resolve-channel?url=...` — resolve YouTube URL → channel ID + name
- `GET /api/fetch-rss?channel_id=...` — fetch & parse RSS → JSON array of videos
- `GET /` — serve static frontend

## User Flow

1. **Add channel**: Paste URL → calls `/api/resolve-channel` → saves to `channels` bucket
2. **Daily refresh**: On first page load of the day, iterate all channels, call `/api/fetch-rss` for each → batch upsert new videos into `videos` bucket. Store `last_refresh_date` in localStorage to avoid re-fetching on subsequent visits the same day.
3. **Browse feed**: List `videos` bucket sorted by `published_at` desc → render video cards
4. **Interact**: Click video → opens YouTube in new tab. Toggle watched/favorited → `PUT /buckets/mine/videos/{id}`
5. **Manage channels**: List/remove channels from sidebar

## Daily Refresh Logic

On app load, React checks `localStorage.getItem("last_refresh_date")`:
- If it's not today → fetch all channels, call `/api/fetch-rss` for each, batch upsert videos, set `last_refresh_date` to today
- If it's today → skip, just load videos from the bucket

This keeps things simple — no cron, no background jobs. You get fresh content once per day when you open the app.

## Running Locally

```bash
# Terminal 1: Firestore emulator
gcloud emulators firestore start --host-port=localhost:9090

# Terminal 2: Go backend
cd standalone-apis/focus-tube
FIRESTORE_EMULATOR_HOST=localhost:9090 go run .
# → http://localhost:8082

# Terminal 3: Vite dev server
cd standalone-apis/focus-tube/frontend
npm run dev
# → http://localhost:5173 (proxies /api and /buckets to :8082)
```

Port: **8082** for Go backend (avoids conflict with game-service:8080 and stock-prices:8081).

## Phase 2: Notes View & Embedded Player

- Video detail page with embedded YouTube player + notes editor alongside
- "Library/Notes" view: filter to videos where `favorited == true` or `notes != ""`
- These are your personal notes that happen to have a YouTube video attached

## Non-Goals (for now)

- Multi-user / real auth
- Search
- Import/export OPML
- Mobile app (web-only for now)
- Background/cron RSS refresh
- Caching/storing video content
