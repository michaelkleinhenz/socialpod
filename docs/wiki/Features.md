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
   - **Start / End date** — the posting window
   - **Posts per day** — target number of posts to publish each day
   - **Min. hours between posts** — the minimum delay between two consecutive
     posts. This takes precedence over posts-per-day: it caps the effective
     count at `floor(24 / minHours)` per day (e.g. 5 posts/day with a 10h
     minimum yields at most 2 posts/day).
   - **Platforms** — target social networks

   Approved photos form a *set*. While the queue is active and inside its
   window, the app automatically picks one approved photo **at random** and
   posts it on the cadence above — spaced by the minimum delay (or an even
   spread across the day) ±60 minutes of random jitter. Nothing is
   pre-scheduled, so you can keep topping the queue up and it keeps posting.
3. Click **Create Queue**.

### Adding and Managing Photos

1. Open the queue from the Convention list.
2. Drag and drop image files onto the upload zone.
3. Each photo becomes a queue item with status **Pending**.

Per-item actions:

| Action | Description |
|---|---|
| **Edit caption** | Type or paste a caption; auto-saved |
| **Approve / Unapprove** | Only approved items are eligible to be posted |
| **AI regenerate** | Re-run AI vision analysis for a fresh caption |
| **Reorder** | Use up/down buttons to adjust publish order |
| **Override platforms** | Override the queue's platform selection for one item |
| **Delete** | Remove the item |

### AI Caption Generation

Click **Analyze all** to send all pending items to the OpenRouter vision model (up to 3 concurrent requests). The page polls every 3 seconds and updates captions as they arrive.

### Posting

Approved photos are posted automatically — there is no bulk "schedule everything"
step. On each queue's cadence the app picks one approved photo at random,
publishes it, and removes it from the approved set so it is never repeated.

- Click **View schedule** in the sticky footer to see the approved count and the
  projected upcoming post times (approximate — each gap carries random jitter).
- Click **Post one now** in that dialog to push a random approved photo out
  immediately instead of waiting for the next slot.

Each published post also appears on the calendar and can be edited individually.
When the approved set empties, posting pauses until you approve more photos.

---

## Instagram Feed

SocialPod displays your Instagram account's published post feed with engagement stats in a unified inbox.

| Tab | Description |
|---|---|
| **Feed** | Your Instagram and Bluesky account post feeds with like and comment counts |

---

## Multitenancy

- The **first user** to register becomes the **admin**.
- Subsequent users are regular users who manage their own posts.
- **Admins** have access to: Dashboard with AI insights, social account management, user management, and application settings.
- Each user's posts and suffixes are isolated by default.
- **Teams**: admins can create teams and assign users. Team members share posts and suffixes scoped to the team.

Team API tokens (format: `st_...`) can be generated from the admin **Teams** page.

---

## Dashboard with AI Insights

The admin dashboard provides an overview of your social media activity with AI-powered insights.

### Dashboard Stats

Navigate to the admin area to see post counts broken down by status (scheduled, published, failed, draft) and platform distribution across all connected networks.

### AI Insights

When an OpenRouter API key is configured, click **Generate Insights** on the dashboard to receive AI-generated analysis of your posting patterns, platform performance, and suggestions for improvement. The analysis respects the configured AI output language.

---

## News Creator

A team plugin for creating "news" style social posts from a headline and article URL. Data is forwarded to a configurable n8n webhook for automated processing.

### Enabling News Creator

1. An admin must enable the `news_creator` plugin for the team in **Admin → Teams → Plugins**.
2. Configure the n8n webhook URL and bearer token in **Admin → Teams → Team Settings → News Creator Integration**.

### Creating a News Post

1. Click **News** in the sidebar.
2. Enter the **Episode Number**, **News Tagline**, and **Article URL**.
3. Optionally click **BGG Import** to auto-populate fields from a BoardGameGeek URL.
4. Upload a cover image (drag-and-drop, clipboard paste, or file browse).
5. Optionally enable **Add as Social Media Posting** to also schedule a social post with the news item.

---

## Episode Creator

A team plugin for podcast episode creation. Supports three episode types — News, Review, and Special — each with its own image overlay watermark.

### Enabling Episode Creator

1. An admin must enable the `episode_creator` plugin for the team in **Admin → Teams → Plugins**.
2. Configure the n8n webhook URL, bearer token, and per-type overlay images in **Admin → Teams → Team Settings → Episode Creator Integration**.

### Creating an Episode

1. Click **Episodes** in the sidebar.
2. Enter the **Episode Number**, select the **Type** (News/Review/Special), set the **Date**, and enter the **Title**.
3. For **Review** episodes, additional fields appear: Game Name and Publisher, Links (Publisher and BGG), Rules, Scene (with AI generation), and Intro Text.
4. Upload a cover image and apply the episode type's overlay watermark in the crop modal.

---

## BGG (BoardGameGeek) Integration

Fetch board game metadata, cover images, AI summaries, and social handles directly from BoardGameGeek URLs.

### Using BGG Import

- From the **News** or **Episode** pages: click the BGG Import button, paste a BGG game URL, and fields auto-populate with the game's tagline, article URL, shownotes, and processed cover image.
- From **Convention Mode**: add items in bulk from BGG URLs — each URL fetches game data, generates an AI caption, letterboxes the cover art with episode-type overlays, and creates a queue item ready for approval.
- The **BGG fetch API** (`/api/bgg/fetch`) returns game metadata, an AI one-sentence summary, suggested hashtags, resolved social handles for publishers/designers/artists (using the local Publisher Handles catalog with AI fallback), and a watermark-processed cover image.

### Admin Configuration

Admins can configure per-team BGG settings:
- **Watermark overlay** and X/Y cover position offsets
- **AI handle lookup** toggle for resolving social handles during imports
- **Scene prompt template** for AI-generated episode scene text

---

## PWA Share Target

SocialPod registers as a Web Share Target, allowing you to share images and videos from other apps directly into SocialPod to schedule a post.

### How to Use

1. Install SocialPod as a PWA (Add to Home Screen in your mobile browser).
2. From any app that supports sharing (Photos, Safari, Chrome, etc.), tap the **Share** button.
3. Select **SocialPod** from the share sheet.
4. SocialPod opens with the shared media pre-loaded. Add a caption, choose platforms, set the schedule, and post.

The Share Target page persists media in a service worker cache, so shared files survive a browser tab being backgrounded.

---

## Mobile Quick Create

A standalone, phone-optimized page for quickly creating posts on mobile devices. Designed to be bookmarked or added to the home screen.

### How to Access

Navigate to `/m/create` on your mobile device. The page loads outside the main app layout for a fast, focused experience.

### Usage

1. Choose a post type: **Post**, **Story**, or **Reel**.
2. Compose your post in the full Post Editor.
3. Save — the editor clears and returns to the type chooser for the next post.

Editor state (post type and draft content) persists in `localStorage`, so closing and reopening the page restores your in-progress post rather than dropping you back at the type chooser.

---

## Team Invites

Invite new users to join your team by email. Invitees receive a link to create their account.

### Sending Invites (Team Admin or Admin)

1. Go to **Admin → My Team** (team admin) or **Admin → Teams → Manage Members** (global admin).
2. Click **Invite**, enter the recipient's email address, and send.
3. The invitee receives an email with a link valid for 7 days.

### Accepting an Invite

1. Open the invite link — it shows the team name and invited email.
2. Enter your name and a password (minimum 8 characters).
3. Click **Join Team** — your account is created and you're logged in immediately.

Global admins can manage all invites from **Admin → Teams → select a team → Manage Members**.

---

## Publisher Handles

An admin-maintained catalog of publisher, designer, and artist names mapped to their social media handles across platforms. Used as a lookup reference during BGG imports.

### Managing Publisher Handles (Admin)

1. Go to **Admin → Publisher Handles**.
2. Click **Add Entry**, enter the name (as it appears on BGG) and fill in handles per platform.
3. During BGG imports, the local catalog is checked first for known names; only unknown names trigger AI handle resolution.

---

## Adobe Express Integration

SocialPod can integrate the [Adobe Express Embed SDK](https://developer.adobe.com/express/embed-sdk/) to let users create images directly inside the post editor. Enable it by entering your Adobe Express Client ID in **Settings → Adobe Express**.

### Browser Configuration

Adobe Express requires popups and cross-origin iframes. If they are blocked:

**Chrome**: Allow pop-ups for your SocialPod domain at `chrome://settings/content/popups` and allow third-party cookies for `[*.]adobe.com`.

**Firefox**: Toggle off Enhanced Tracking Protection for the site, or allow popups when the browser notification appears.

**Brave**: Set Shields down for the site, or allow all cookies and disable Block pop-ups under Brave Shields.

> Adobe Express requires users to sign in with an Adobe account when first exporting a design.

### Troubleshooting: Login succeeds but the editor stays logged out

If the login popup opens, you authenticate, and then the Adobe Express editor returns to the same screen but still shows you as logged out, the Adobe Embed SDK has lost its session. This is a known Adobe SDK issue (`CCXSDK-6179`) caused by browsers blocking third-party cookies/storage for the embedded Adobe iframe.

The integration code itself does not store the Adobe session — it is managed entirely inside Adobe’s iframe — so the fix is in the browser:

- **Chrome:** `chrome://settings/cookies` → turn off “Block third-party cookies” or add exceptions for your SocialPod domain and `[*.]adobe.com`.
- **Safari:** Settings → Privacy → disable “Prevent Cross-Site Tracking” and “Block All Cookies”.
- **Firefox:** Click the shield icon in the address bar on SocialPod and turn off Enhanced Tracking Protection.
- **Brave:** Lower Shields for the SocialPod domain and allow third-party cookies.

Also verify that the **Redirect URI pattern** in your Adobe Developer Console project still matches your current SocialPod URL.
