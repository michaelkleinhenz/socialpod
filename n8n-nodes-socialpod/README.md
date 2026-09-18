# n8n-nodes-socialpod

This is an [n8n](https://n8n.io/) community node that lets you automate [SocialPod](https://github.com/michaelkleinhenz/socialpod) — a self-hosted social media scheduler supporting Bluesky, Instagram, Twitter/X, Mastodon, Threads, LinkedIn, and YouTube.

[n8n](https://n8n.io/) is a [fair-code licensed](https://docs.n8n.io/reference/license/) workflow automation platform.

## Installation

### Community Nodes (recommended)

1. Open **Settings > Community Nodes** in your n8n instance.
2. Click **Install a community node**.
3. Enter `n8n-nodes-socialpod` and click **Install**.

### Manual installation

```bash
cd ~/.n8n/nodes
npm install n8n-nodes-socialpod
```

Restart n8n after installing.

## Setup

1. In your SocialPod instance, go to **Profile** and generate an API token.
2. In n8n, add a new **SocialPod API** credential with:
   - **API URL** — base URL of your SocialPod instance (e.g. `https://socialpod.example.com`)
   - **API Token** — the `sm_...` token from your profile page

## Resources and Operations

| Resource    | Operations                                             |
|-------------|--------------------------------------------------------|
| **Post**    | Create, Get, List, Update, Delete, Reschedule, Retry   |
| **Footer**  | Create, List, Update, Delete                           |
| **Account** | List                                                   |
| **Mention** | Create, List, Update, Delete, Export, Import            |
| **Watermark** | List, Delete                                         |
| **AI Text** | Generate                                               |

### Post features

- Schedule posts across all seven platforms simultaneously
- Attach images via binary data from previous nodes or by URL
- Per-platform content overrides (different captions per platform)
- Per-platform footer and account selection
- Support for Instagram Reels and Stories
- First-comment support
- Tags for internal organization

### Mention features

- Manage a directory of publisher/person handles across platforms
- Bulk import/export as JSON

## Compatibility

- **n8n**: 1.0+
- **Node.js**: 20+

## License

[MIT](LICENSE)
