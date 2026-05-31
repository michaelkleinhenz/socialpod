# Features

---

## Suffix Management

Suffixes are text snippets **automatically appended** to a post when it is published, without counting against the character limit visible in the editor.

### Creating Suffixes

1. Click **Suffixes** in the sidebar.
2. Click **New Suffix**, give it a name and content (e.g. `🌐 mysite.com`).
3. Click **Create**.

### Using Suffixes in Posts

When composing a post, suffix dropdowns appear per platform. The character counter deducts the suffix length in real time so you always see accurate remaining characters.

Suffixes are stored by reference — updating a suffix affects all future posts that use it.

---

## Built-in Image Editor

Every post image can be edited in the browser using the built-in image editor (powered by [Filerobot Image Editor](https://github.com/scaleflex/filerobot-image-editor)). Click the pencil icon on any attached image to open it.

### Available Tools

| Tab | Description |
|---|---|
| **Adjust** | Brightness, contrast, saturation, exposure, and more |
| **Annotate** | Text overlays, shapes, arrows, and freehand drawing |
| **Watermark** | Place admin-uploaded watermark images on the photo |
| **Filters** | Apply preset color filters |
| **Finetune** | Fine-grained colour and tone adjustments |
| **Resize** | Change image dimensions |

### Crop Presets

| Preset | Ratio |
|---|---|
| Square | 1:1 |
| Story | 9:16 |
| Landscape | 16:9 |
| Portrait | 4:5 |

---

## Watermarks

Admins can manage a library of watermark images available to all users inside the image editor.

### Managing Watermarks (Admin)

1. Navigate to **Watermarks** in the sidebar.
2. Optionally enter a name, click **Upload**, and select an image (PNG, JPEG, GIF, or WebP).
3. The watermark is immediately available in the image editor.
4. Click the trash icon to remove a watermark.

### Applying Watermarks (Users)

1. Open a post image and click the pencil icon.
2. Select the **Watermark** tab.
3. Click a watermark from the gallery to place it on the image.
4. Drag, resize, or reposition it, then click **Save**.

---

## Per-Platform Text Customization

Write different text for each platform when posting to multiple networks simultaneously.

### How to Use

1. In the post editor, select two or more platforms.
2. Toggle **Customize per platform**.
3. A separate text field appears for each platform. Leave a field empty to fall back to the shared text.

### Character Limits

| Platform | Limit |
|---|---|
| Bluesky | 300 |
| X (Twitter) | 280 |
| Threads | 500 |
| Mastodon | 500 |
| Instagram | 2,200 |
| LinkedIn | 3,000 |
| YouTube | 5,000 (description) |

---

## Mentions / @ Autocomplete

Maintain a list of frequently mentioned accounts with per-platform handles. Typing `@` in the post editor triggers an autocomplete dropdown.

### Managing Mentions

1. Click **Mentions** in the sidebar.
2. Click **New Mention**, enter a display name, and fill in the handle for each platform.
3. Click **Create**.

### Using @ Autocomplete

- Type `@` anywhere in a post text area to see the dropdown.
- In a **per-platform text area**, selecting a mention inserts that platform's handle.
- In the **shared text area**, selecting a mention automatically enables per-platform mode and inserts the correct handle in each field.

Mentions are scoped to your user or team.

---

## AI Text Generation

When an OpenRouter API key is configured, a magic-wand button appears in the post editor for rewriting or improving post text.

### Enabling AI Text Generation

1. Log in as an admin and go to **Settings → AI / OpenRouter**.
2. Enter your OpenRouter API key and select a model.
3. Optionally choose an **AI Output Language** (defaults to English). Supported languages include German, French, Spanish, Italian, Dutch, Portuguese, Brazilian Portuguese, Japanese, Korean, Chinese, Arabic, Polish, Swedish, Norwegian, Danish, and Finnish.
4. Save. The wand button becomes available to all users immediately.

The selected language applies to all AI output: post copy, convention captions, and admin dashboard insights.

---

## Convention Mode

Convention Mode is designed for events (trade shows, gaming conventions, fan expos) where you shoot many photos and want to drip-publish them over the following days.

> **Requires**: An OpenRouter API key in **Settings → AI / OpenRouter** for AI caption generation. Queues can be managed and scheduled manually without it.

### Creating a Queue

1. Click **Convention** in the sidebar.
2. Click **New Queue** and configure:
   - **Name** — e.g. "Essen SPIEL 2026"
   - **Convention URL** (optional) — website or hashtag URL
   - **Hashtags** — prepended/appended to every post
   - **Start / End date** — the drip window
   - **Posts per day** — how many posts to publish each day
   - **Time slots** — e.g. `09:00`, `13:00`, `18:00`
   - **Platforms** — target social networks
3. Click **Create Queue**.

### Adding and Managing Photos

1. Open the queue from the Convention list.
2. Drag and drop image files onto the upload zone.
3. Each photo becomes a queue item with status **Pending**.

Per-item actions:

| Action | Description |
|---|---|
| **Edit caption** | Type or paste a caption; auto-saved |
| **Approve / Unapprove** | Only approved items are scheduled |
| **AI regenerate** | Re-run AI vision analysis for a fresh caption |
| **Reorder** | Use up/down buttons to adjust publish order |
| **Override platforms** | Override the queue's platform selection for one item |
| **Delete** | Remove the item |

### AI Caption Generation

Click **Analyze all** to send all pending items to the OpenRouter vision model (up to 3 concurrent requests). The page polls every 3 seconds and updates captions as they arrive.

### Scheduling

1. Click **Preview & schedule** in the sticky footer.
2. Review each approved item with its assigned publish time.
3. Click **Schedule** to convert all approved items into scheduled posts.

Scheduled posts appear on the calendar and can be edited individually.

---

## Instagram Inbox

SocialPod displays Instagram comments and direct messages via Meta webhooks in a unified inbox.

### Prerequisites

- `APP_URL` must be publicly accessible over HTTPS.
- An Instagram account connected in **Accounts**.

### Configuring Meta Webhooks

1. In your Meta app dashboard, go to **Webhooks**.
2. Add a subscription for the **Instagram** object.
3. Set the **Callback URL** to:
   ```
   https://your-domain.com/api/webhooks/instagram
   ```
4. Set the **Verify Token** to any string.
5. Subscribe to the `comments` and `messages` fields.

### Using the Inbox

| Tab | Description |
|---|---|
| **Comments** | Instagram comments with unread counts and reply support |
| **Direct Messages** | DMs with unread tracking and reply support |
| **Feed** | Your Instagram account's own post feed |

Messages are marked as read when opened. Click **Reply** to respond directly from SocialPod.

---

## Multitenancy

- The **first user** to register becomes the **admin**.
- Subsequent users are regular users who manage their own posts.
- **Admins** have access to: Dashboard with AI insights, social account management, user management, and application settings.
- Each user's posts and suffixes are isolated by default.
- **Teams**: admins can create teams and assign users. Team members share posts and suffixes scoped to the team.

Team API tokens (format: `st_...`) can be generated from the admin **Teams** page.

---

## Adobe Express Integration

SocialPod can integrate the [Adobe Express Embed SDK](https://developer.adobe.com/express/embed-sdk/) to let users create images directly inside the post editor. Enable it by entering your Adobe Express Client ID in **Settings → Adobe Express**.

### Browser Configuration

Adobe Express requires popups and cross-origin iframes. If they are blocked:

**Chrome**: Allow pop-ups for your SocialPod domain at `chrome://settings/content/popups` and allow third-party cookies for `[*.]adobe.com`.

**Firefox**: Toggle off Enhanced Tracking Protection for the site, or allow popups when the browser notification appears.

**Brave**: Set Shields down for the site, or allow all cookies and disable Block pop-ups under Brave Shields.

> Adobe Express requires users to sign in with an Adobe account when first exporting a design.
