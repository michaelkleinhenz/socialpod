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

# Export mentions as JSON
curl http://localhost:8080/api/mentions/export \
  -H "Authorization: Bearer sm_..."

# Import mentions from JSON
curl -X POST http://localhost:8080/api/mentions/import \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '[{"name":"Acme Corp","handles":{"bluesky":"@acme.bsky.social"}}]'

# Delete a mention
curl -X DELETE http://localhost:8080/api/mentions/{id} \
  -H "Authorization: Bearer sm_..."
```

---

## Password & API Token

```bash
# Update your password
curl -X PUT http://localhost:8080/api/auth/password \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"currentPassword":"old-password","newPassword":"new-password"}'

# Generate/reveal a user API token
curl -X POST http://localhost:8080/api/auth/api-token \
  -H "Authorization: Bearer sm_..."
```

---

## Dashboard

```bash
# Get dashboard stats (post counts by status, platform breakdown)
curl http://localhost:8080/api/dashboard/stats \
  -H "Authorization: Bearer sm_..."

# Get AI-generated dashboard insights (requires OpenRouter key)
curl -X POST http://localhost:8080/api/dashboard/ai-insights \
  -H "Authorization: Bearer sm_..."
```

---

## News Creator

News Creator is a team plugin that creates news-style posts from a headline, article URL, and image. Data is forwarded to a configurable n8n webhook and optionally creates a social post.

```bash
curl -X POST http://localhost:8080/api/news/submit \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"episodeNumber":42,"newsTagline":"Breaking news!","articleUrl":"https://example.com/news","shownotes":"Optional notes","addSocialPosting":true,"platforms":["bluesky"],"scheduledAt":"2026-06-01T09:00:00Z","status":"scheduled","contentOverrides":{"bluesky":"Custom text"}}' \
  -F "image=@news.jpg"
```

---

## Episode Creator

Episode Creator is a team plugin for podcast episode creation. Supports three episode types (`news`, `review`, `special`). Review-type episodes include additional game metadata fields.

```bash
curl -X POST http://localhost:8080/api/episode/submit \
  -H "Authorization: Bearer sm_..." \
  -F 'data={"episodeNumber":42,"episodeTitle":"My Episode","episodeType":"review","episodeDate":"2026-06-01T00:00:00Z","summary":"Episode summary","gameNamePublisher":"Great Game","linkPublisher":"https://publisher.com","linkBGG":"https://boardgamegeek.com/boardgame/123","rules":"Game rules","scene":"Scene description","introText":"Intro","addSocialPosting":true,"platforms":["bluesky"],"scheduledAt":"2026-06-01T09:00:00Z","status":"scheduled"}' \
  -F "image=@cover.jpg"
```

---

## BGG (BoardGameGeek)

Fetches board game metadata from BGG, downloads and processes the cover image with optional watermarks and overlays, generates AI summaries and hashtags, and resolves social media handles for publishers/designers/artists.

```bash
# Fetch game info from a BGG URL
curl "http://localhost:8080/api/bgg/fetch?url=https://boardgamegeek.com/boardgame/174430/glen-more-ii-chronicles&episodeType=review" \
  -H "Authorization: Bearer sm_..."
# Returns: game metadata, AI summary, suggested hashtags, social handles, processed image URL

# Get team BGG settings
curl http://localhost:8080/api/team/settings \
  -H "Authorization: Bearer sm_..."
```

---

## Team Setup & Plugin Management

```bash
# Team admin creates their own team
curl -X POST http://localhost:8080/api/team/setup \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"My Team"}'
```

### Admin-only team endpoints

```bash
# Get/update team plugins (enable/disable features per team)
curl http://localhost:8080/api/admin/teams/{id}/plugins \
  -H "Authorization: Bearer sm_..."
curl -X PUT http://localhost:8080/api/admin/teams/{id}/plugins \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '["news_creator","episode_creator"]'

# Get/update team BGG settings (watermarks, overlays, n8n webhook URLs, etc.)
curl http://localhost:8080/api/admin/teams/{id}/settings \
  -H "Authorization: Bearer sm_..."

# Generate team API token (format: st_...)
curl -X POST http://localhost:8080/api/admin/teams/{id}/token \
  -H "Authorization: Bearer sm_..."
```

---

## Team Invites

Invite users to a team by email. Invitees receive a link to create their account. Tokens expire after 7 days.

```bash
# Get invite info (public — used by invitee to see team name before signing up)
curl "http://localhost:8080/api/invites/info?token=abc123..."

# Accept an invite (public — creates account and returns JWT)
curl -X POST http://localhost:8080/api/invites/accept \
  -H "Content-Type: application/json" \
  -d '{"token":"abc123...","name":"John Doe","password":"securepassword"}'

# Admin creates an invite
curl -X POST http://localhost:8080/api/admin/teams/{id}/invites \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"email":"friend@example.com"}'

# Admin lists/deletes invites
curl http://localhost:8080/api/admin/teams/{id}/invites \
  -H "Authorization: Bearer sm_..."

# Team admins can also manage their own invites
curl -X POST http://localhost:8080/api/team/invites \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"email":"new-team-member@example.com"}'
curl http://localhost:8080/api/team/invites \
  -H "Authorization: Bearer sm_..."
```

---

## Team Admin Endpoints

Team admins can manage their own team's accounts and members (scoped to their team):

```bash
# List team accounts
curl http://localhost:8080/api/team/accounts \
  -H "Authorization: Bearer st_..."

# Add a Bluesky account for the team
curl -X POST http://localhost:8080/api/team/accounts/bluesky \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"identifier":"handle.bsky.social","appPassword":"xxxx-xxxx-xxxx-xxxx"}'

# Add a Twitter account for the team
curl -X POST http://localhost:8080/api/team/accounts/twitter \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"apiKey":"...","apiKeySecret":"...","accessToken":"...","accessTokenSecret":"..."}'

# Add a Mastodon account for the team
curl -X POST http://localhost:8080/api/team/accounts/mastodon \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"instance":"mastodon.social","accessToken":"..."}'

# Add a Threads account for the team
curl -X POST http://localhost:8080/api/team/accounts/threads \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"accessToken":"..."}'

# Add a LinkedIn account for the team
curl -X POST http://localhost:8080/api/team/accounts/linkedin \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"authorId":"..."}'

# Get OAuth auth URLs for team-scoped accounts
curl http://localhost:8080/api/team/instagram/auth-url \
  -H "Authorization: Bearer st_..."

# Toggle account active/inactive
curl -X PATCH http://localhost:8080/api/team/accounts/{id}/toggle \
  -H "Authorization: Bearer st_..."

# Delete a team account
curl -X DELETE http://localhost:8080/api/team/accounts/{id} \
  -H "Authorization: Bearer st_..."

# List team members
curl http://localhost:8080/api/team/members \
  -H "Authorization: Bearer st_..."

# Add a member by email
curl -X POST http://localhost:8080/api/team/members \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"email":"existing-user@example.com"}'

# Remove a member
curl -X DELETE http://localhost:8080/api/team/members/{userId} \
  -H "Authorization: Bearer st_..."

# Update team BGG settings
curl -X PUT http://localhost:8080/api/team/settings \
  -H "Authorization: Bearer st_..." \
  -H "Content-Type: application/json" \
  -d '{"bggWatermarkId":"...","newsCreatorUrl":"https://..."}'
```

---

## Publisher Handles

Admin-only catalog of publisher/designer/artist names mapped to their social media handles. Used as a lookup reference during BGG imports to avoid repeated AI handle resolution.

```bash
# List all entries
curl http://localhost:8080/api/admin/publisher-handles \
  -H "Authorization: Bearer sm_..."

# Create an entry
curl -X POST http://localhost:8080/api/admin/publisher-handles \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Games","handles":{"bluesky":"@acme.bsky.social","instagram":"@acmegames","twitter":"@acme_games"}}'

# Update an entry
curl -X PUT http://localhost:8080/api/admin/publisher-handles/{id} \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"handles":{"bluesky":"@new-handle.bsky.social"}}'

# Delete an entry
curl -X DELETE http://localhost:8080/api/admin/publisher-handles/{id} \
  -H "Authorization: Bearer sm_..."
```

---

## Image Upload

```bash
# Upload a single image
curl -X POST http://localhost:8080/api/upload \
  -H "Authorization: Bearer sm_..." \
  -F "image=@photo.jpg"
# Returns: {"url": "/api/uploads/1234567890.jpg", "filename": "1234567890.jpg"}

# Upload from a remote URL
curl -X POST http://localhost:8080/api/upload-from-url \
  -H "Authorization: Bearer sm_..." \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/photo.jpg"}'
```

Include returned URLs in the `imageUrls` array of the post `data` field, or attach binary files directly as `images` fields.
