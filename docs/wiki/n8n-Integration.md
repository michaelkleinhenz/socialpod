# n8n Integration

SocialPod ships with a native **n8n community node** in the `n8n-nodes-socialpod/` directory. It handles authentication automatically and supports all post and suffix operations.

> **Platform support**: The n8n node currently supports **Bluesky** and **Instagram**. X (Twitter), Mastodon, Threads, and LinkedIn can be scheduled via the [[REST API]] directly.

## Supported Operations

| Resource | Operations |
|---|---|
| **Post** | Create, Get, List, Update, Delete, Reschedule |
| **Suffix** | Create, List, Update, Delete |

The **Post → Create** and **Post → Update** operations include a **Post Type** field: `Post` (default), `Reel`, and `Story`.

## Installation

The node ships with a pre-built `dist/` directory — no compilation needed on the target machine.

> **Important**: Always use `npm install --omit=dev` when installing on the n8n server. A full `npm install` pulls in `n8n-workflow` and its transitive native module `isolated-vm`, which requires Node >= 22 to compile. Since n8n provides `n8n-workflow` at runtime, only `--omit=dev` is needed.

### Option A — Local directory (self-hosted n8n)

1. Copy the node to n8n's custom nodes directory:

   | n8n setup | Custom nodes path |
   |---|---|
   | npm global | `~/.n8n/custom` |
   | Docker | Mount a volume at `/home/node/.n8n/custom` |
   | n8n Desktop | `~/.n8n/custom` |

   ```bash
   mkdir -p ~/.n8n/custom
   cp -r n8n-nodes-socialpod ~/.n8n/custom/
   cd ~/.n8n/custom/n8n-nodes-socialpod
   npm install --omit=dev
   ```

2. Restart n8n. The SocialPod node will appear in the node palette.

### Option B — npm link (development)

Requires Node >= 22:

```bash
cd n8n-nodes-socialpod
npm install
npm run build
npm link

# In your n8n installation directory:
npm link n8n-nodes-socialpod
```

Restart n8n after linking.

### Option C — Docker Compose (recommended for production)

```yaml
services:
  n8n:
    image: n8nio/n8n
    volumes:
      - ./n8n-nodes-socialpod:/home/node/.n8n/custom/n8n-nodes-socialpod
    environment:
      - N8N_CUSTOM_EXTENSIONS=/home/node/.n8n/custom
```

On the host, install runtime dependencies only:

```bash
cd n8n-nodes-socialpod
npm install --omit=dev
```

The pre-built `dist/` is already included — no build step required.

## Configuring the Credential

1. In n8n, go to **Credentials → New Credential → SocialPod API**.
2. Enter:
   - **API URL** — base URL of your SocialPod instance (e.g. `https://socialpod.example.com`)
   - **API Token** — your token from the SocialPod Profile page (`sm_...`) or a team token (`st_...`)
3. Click **Save**. n8n will test the credential against `/api/auth/me`.

## Example Workflow

A workflow that creates a scheduled post every weekday at 9 AM:

```
[Cron: Mon–Fri 09:00] → [SocialPod: Post → Create]
```

In the **SocialPod** node, configure:
- **Resource**: Post
- **Operation**: Create
- **Content**: `{{ $json.content }}` (or a static string)
- **Platforms**: Bluesky, Instagram
- **Scheduled At**: `{{ new Date().toISOString() }}`
- **Status**: Scheduled
