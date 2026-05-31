# REST API

SocialPod exposes a REST API for external scheduling and integration. Authenticate with either a JWT token (from login) or an API token.

## Generate an API Token

1. Go to **Profile** (click your name in the sidebar).
2. Click **Generate API Token**.
3. Copy the token (format: `sm_...`).

Team tokens (format: `st_...`) are generated from the admin **Teams** page.

## Authentication

Include the token in every request:

```
Authorization: Bearer sm_your_api_token_here
```

## Health Check

```bash
curl http://localhost:8080/api/health
# Returns: {"status": "ok"}
```

---

## Posts

### Post Data Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `content` | string | Yes* | Post text (*not required for Stories) |
| `platforms` | array | Yes | Any of `"bluesky"`, `"instagram"`, `"twitter"`, `"mastodon"`, `"threads"`, `"linkedin"`, `"youtube"` |
| `postType` | string | No | `post` (default), `story`, or `reel` |
| `scheduledAt` | string | Yes | ISO 8601 datetime (e.g. `2025-06-01T09:00:00Z`) |
| `status` | string | No | `scheduled` (default) or `draft` |
| `imageUrls` | array | No | Pre-uploaded image/video URLs |
| `firstComment` | string | No | Posted as the first comment after publishing |
| `suffixIds` | object | No | `{"bluesky":"<id>","instagram":"<id>"}` |
| `accountIds` | object | No | `{"bluesky":"<id>"}` — specific account to use per platform |
| `contentOverrides` | object | No | Per-platform caption overrides; missing platforms fall back to `content` |
| `tags` | array | No | Tags for internal organisation |

> `story` and `reel` post types apply differently per platform:
> - **Instagram**: `story` posts a Story (media required), `reel` posts a Reel (MP4 video required).
> - **YouTube**: `reel` posts a YouTube Short (video required; the title is prefixed with `#Shorts` automatically). `post` uploads a regular video.
> - `content` is used as the caption for Instagram Reels and as the YouTube video description. It is ignored for Instagram Stories.

### Examples

```bash
# Create a scheduled post
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Hello from the API!","platforms":["bluesky"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}'

# Create a post with images
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Photo post","platforms":["instagram"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@photo.jpg"

# Create an Instagram Story
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"","platforms":["instagram"],"postType":"story","scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@story.jpg"

# Create an Instagram Reel
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Check out this reel!","platforms":["instagram"],"postType":"reel","scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@reel.mp4"

# Cross-post with per-platform text overrides
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Default caption","platforms":["bluesky","instagram","twitter","mastodon","threads","linkedin","youtube"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled","contentOverrides":{"twitter":"Short tweet (280 chars max)","mastodon":"Fediverse post","linkedin":"Professional update","youtube":"Full video description"}}'

# Upload a YouTube video
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"My new video description","platforms":["youtube"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@video.mp4"

# Upload a YouTube Short
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Short clip caption","platforms":["youtube"],"postType":"reel","scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled"}' \
  -F "images=@short.mp4"

# Create a post with suffixes
curl -X POST http://localhost:8080/api/posts \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Check this out","platforms":["bluesky","instagram"],"scheduledAt":"2025-06-01T09:00:00Z","status":"scheduled","suffixIds":{"bluesky":"<suffix-id>","instagram":"<suffix-id>"}}'

# List posts (optional: start, end, status, platform query params)
curl "http://localhost:8080/api/posts?start=2025-01-01T00:00:00Z&end=2025-12-31T23:59:59Z&status=scheduled" \
  -H "Authorization: Bearer sm_..."

# Get a single post
curl http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..."

# Update a post
curl -X PUT http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"content":"Updated content"}'

# Reschedule a post
curl -X PATCH http://localhost:8080/api/posts/{id}/reschedule \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"scheduledAt":"2025-06-02T10:00:00Z"}'

# Delete a post
curl -X DELETE http://localhost:8080/api/posts/{id} \
  -H "Authorization: Bearer sm_..."

# Retry a failed post
curl -X POST http://localhost:8080/api/posts/{id}/retry \
  -H "Authorization: Bearer sm_..."
```

---

## Accounts

```bash
# List active social accounts visible to the authenticated user/team
curl http://localhost:8080/api/accounts \
  -H "Authorization: Bearer sm_..."
```

---

## Suffixes

```bash
# List suffixes
curl http://localhost:8080/api/suffixes \
  -H "Authorization: Bearer sm_..."

# Create a suffix
curl -X POST http://localhost:8080/api/suffixes \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"Website footer","content":"🌐 mysite.com"}'

# Update a suffix
curl -X PUT http://localhost:8080/api/suffixes/{id} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"content":"🌐 mysite.com | follow for more"}'

# Delete a suffix
curl -X DELETE http://localhost:8080/api/suffixes/{id} \
  -H "Authorization: Bearer sm_..."
```

---

## Mentions

```bash
# List mentions
curl http://localhost:8080/api/mentions \
  -H "Authorization: Bearer sm_..."

# Create a mention
curl -X POST http://localhost:8080/api/mentions \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Corp","handles":{"bluesky":"@acme.bsky.social","twitter":"@AcmeCorp","mastodon":"@acme@mastodon.social"}}'

# Update a mention
curl -X PUT http://localhost:8080/api/mentions/{id} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"handles":{"bluesky":"@acme-new.bsky.social"}}'

# Delete a mention
curl -X DELETE http://localhost:8080/api/mentions/{id} \
  -H "Authorization: Bearer sm_..."
```

---

## Convention Mode

```bash
# List queues
curl http://localhost:8080/api/convention/queues \
  -H "Authorization: Bearer sm_..."

# Create a queue
curl -X POST http://localhost:8080/api/convention/queues \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"Essen SPIEL 2026","startDate":"2026-10-27T00:00:00Z","endDate":"2026-11-10T00:00:00Z","postsPerDay":3,"timeSlots":["09:00","13:00","18:00"],"platforms":["bluesky","instagram"],"hashtags":["#EssenSPIEL"]}'

# Get a queue (includes items)
curl http://localhost:8080/api/convention/queues/{id} \
  -H "Authorization: Bearer sm_..."

# Add a photo item
curl -X POST http://localhost:8080/api/convention/queues/{id}/items \
  -H "Authorization: Bearer sm_..." \
  -F "image=@photo.jpg"

# Update an item
curl -X PUT http://localhost:8080/api/convention/queues/{id}/items/{iid} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"caption":"Amazing booth at #EssenSPIEL!","status":"approved"}'

# Reorder items
curl -X POST http://localhost:8080/api/convention/queues/{id}/reorder \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"order":["<item-id-1>","<item-id-2>","<item-id-3>"]}'

# Analyze a single item (AI caption generation)
curl -X POST http://localhost:8080/api/convention/queues/{id}/items/{iid}/analyze \
  -H "Authorization: Bearer sm_..."

# Analyze all pending items
curl -X POST http://localhost:8080/api/convention/queues/{id}/analyze-all \
  -H "Authorization: Bearer sm_..."

# Preview the drip schedule (dry run)
curl http://localhost:8080/api/convention/queues/{id}/preview \
  -H "Authorization: Bearer sm_..."

# Schedule all approved items
curl -X POST http://localhost:8080/api/convention/queues/{id}/schedule \
  -H "Authorization: Bearer sm_..."

# Delete an item
curl -X DELETE http://localhost:8080/api/convention/queues/{id}/items/{iid} \
  -H "Authorization: Bearer sm_..."

# Delete a queue
curl -X DELETE http://localhost:8080/api/convention/queues/{id} \
  -H "Authorization: Bearer sm_..."
```

---

## Image Upload

```bash
curl -X POST http://localhost:8080/api/upload \
  -H "Authorization: Bearer sm_..." \
  -F "image=@photo.jpg"
# Returns: {"url": "/api/uploads/1234567890.jpg", "filename": "1234567890.jpg"}
```

Include returned URLs in the `imageUrls` array of the post `data` field, or attach binary files directly as `images` fields.
