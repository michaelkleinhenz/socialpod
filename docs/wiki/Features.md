# Features

---

## Footer Management

Footers are text snippets **automatically appended** to a post when it is published, without counting against the character limit visible in the editor.

### Creating Footers

1. Click **Footers** in the sidebar.
2. Click **New Footer**, give it a name and content (e.g. `🌐 mysite.com`).
3. Click **Create**.

### Using Footers in Posts

When composing a post, footer dropdowns appear per platform. The character counter deducts the footer length in real time so you always see accurate remaining characters.

Footers are stored by reference — updating a footer affects all future posts that use it.

---

## Media Library

**Post Tools → Media Library** holds the team's material: images, videos, sounds (mp3, m4a, aac, wav, ogg), fonts (.ttf/.otf), colours and design templates. Each entry has a name, a description and tags; search by words and tags, and narrow the list with the kind filter (All, Images, Watermarks, Videos, Audio, Fonts, Colours, Templates). Fonts are shown as a sample set in them, colours as swatches, and videos and sounds play in their cards.

- **Add Colour** stores a hex value with a description of how to use it ("headlines and badges"). Library colours are the first swatches in the image editor.
- Library fonts are offered in the image editor ahead of the system fonts.
- Descriptions matter: AI agents pick their material by them (see [[Content Agent]]).

A file stays stored as long as the library (or anything else) uses it; deleting an entry frees it otherwise.

### Watermarks

Watermarks are library images marked as watermarks: tick the checkbox in an image's edit form, or upload in the **Watermarks** view. They are what the team's overlay settings (BGG, news and episode overlays), convention queues and the editors' overlay pickers offer. Unmarking keeps the image; deleting removes it. The old `/watermarks` page now opens the library's Watermarks view.

---

## Image Editor

Click the pencil on any post image (or on a convention queue item) to open SocialPod's own full-screen editor. **Create Image** in the post editor, News and Episode forms opens it on a blank canvas instead (4:5 for a post, 9:16 for a story, 1:1 for News and Episodes).

### Tools

| Tool | Description |
|---|---|
| **Crop** | Crop with aspect presets (free, original, 1:1, 4:5, 9:16, 16:9); re-cropping never shifts the layers |
| **Canvas** | On a blank canvas instead of Crop and Filters: format (1:1, 4:5, 9:16, 16:9) and background colour |
| **Filters** | Filter presets plus brightness, contrast and saturation |
| **Templates** | Built-in SocialPod templates, the team's templates and your saved drafts |
| **Media** | Add images (or watermarks, with the Watermarks filter) from the media library, or upload one into it |
| **Text** | Type directly on the picture; font, size, alignment, colour, bold, outline, shadow |
| **Shapes** | Box, circle/ellipse and triangle with fill, border, corners and opacity |
| **Brush** | Paint strokes with size, feather, colour and opacity, e.g. soft shading behind text |

Image layers take rounded corners, a border (solid, dashed, dotted, double, glow, offset), opacity and a crop of their own; every layer takes a blend mode (multiply, screen, overlay, ...). **Replace** (or pasting an image into a template's placeholder) swaps a picture while keeping its frame.

### Working with layers

- The **Objects** list on the right shows the layers top first; drag rows to restack them.
- Moving a layer snaps it to the edges and centres of the page and of other layers, and to equal spacing, with guide lines. The magnet button turns snapping off; hold **Alt** to skip it for one drag.
- Arrow keys nudge the selected layer (Shift for ten steps). Undo/redo with Ctrl/Cmd+Z and Ctrl/Cmd+Shift+Z.
- For 9:16 designs a phone button shows Instagram's **safe zones**: the areas a reel's or story's own interface covers. They are never exported.

### Phones

On screens up to 768 px wide the editor switches to a phone layout: a bottom bar with the add tools (or the selected object's tools), one control at a time in a tray over the picture, bottom sheets for templates, media and the objects list, and a small toolbar under the selected object (duplicate, up/down, delete). Two fingers pinch, turn and move the selected object.

### Templates

- **SocialPod templates** are built in: headline, breaking news, spotlight, review score, new episode, top 5, quote, giveaway, coming soon, game night and more, each in 1:1, 4:5 and 9:16. They use plain colours and the built-in fonts, ready to restyle; their pictures are placeholders to replace or paste over.
- **Team templates** are designs kept in the media library. **New Template** on the Media Library page opens the editor on a blank canvas (still or video); the brush on a template card edits its design.
- On a blank canvas a template replaces the design; on a photo its layers are laid over the crop.

### Drafts

**Save draft** (Ctrl/Cmd+S) keeps unfinished editor work, photo and all layers, under a name. Drafts are listed at the top of the Templates tab under **Yours**; picking one restores it exactly. Drafts are personal.

---

### Carousels in the post editor

A post with several images is a carousel. In the post editor, drag the images into the order they
should appear (on a phone, hold one a moment, then drag). The **Photo** and **Create** tiles after the
images add another photo, or a page designed in the image editor, such as a front or back page; drag
it into place afterwards.

## Video Editor

**Create Video** in the post editor (reels and stories) opens the editor in video mode on a blank 9:16 canvas. A video has a length (3–90 s) and optional music; every layer can be animated.

- **Clips**: add videos from the media library (Media → Clips) or upload one; clips move, resize, turn and take corners, borders and blend modes like images, and have their own trim and volume. Uploads that aren't kept in the library are temporary and cleaned up automatically.
- **Music**: a sound from the library or an upload, with start, trim, volume and fades (Animate tab with nothing selected).
- **Animations** (Animate tab): each layer gets a time span on the timeline, an enter effect (fade, slides, zoom, pop, wipes, typewriter, ...), a loop effect (pulse, float, wiggle, slow zoom, ...) and an exit effect.
- **Keyframes**: add a key at the playhead and set the layer's position, size, rotation and opacity there, with an ease to the next key. Once a layer has keys, dragging or resizing it on the picture sets the key at the playhead. Keys show as diamonds on the layer's bar and can be dragged to retime them.
- **Timeline**: play/pause (Space), repeat, scrub, drag bars to move or trim layers and music.
- **Reel templates** (kinetic intro, countdown reveal, top 3, review verdict, podcast clip, versus, save the date, photo recap, ...) are fully animated scene by scene. Template tiles play on hover.
- **Save template** stores the design with its length, music and animations as a video template in the media library.
- **Export video** renders an MP4 (H.264 + AAC) in the browser and adds it to the post. Browsers without an AAC encoder of their own get one as WebAssembly; a browser without H.264 encoding can't export.

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
2. Drag and drop image files onto one of the three upload zones:
   - **One post per photo**: each photo becomes a queue item with status **Pending**.
   - **One carousel post**: all the photos become a single carousel item.
   - **Gallery**: the photos wait in the queue's gallery until you decide what they become.

### The Gallery

Upload a whole batch to the gallery when you'd rather sort the photos once you see them together. The
gallery appears under the upload zones. Tap photos to pick them (or open one large with its expand
button), then:

- **Single post(s)**: each picked photo becomes its own **Pending** item, exactly as if it had been
  uploaded directly.
- **Carousel…**: the picked photos go into a carousel, in the order you picked them (the numbers on
  the tiles). Start a new carousel, or add them to the end of a carousel already in the queue that
  isn't scheduled yet.

A carousel's pages show as a strip under its picture until it is scheduled. Drag them into another
order, remove one with its ×, or use the **+** before the first or after the last page to add a front
or back page: upload a photo, or **Create in editor** to design one (e.g. a title or "follow us" page).

A moved photo leaves the gallery. Photos you never move can be deleted from the photo view; deleting
the queue deletes its gallery too.

Per-item actions:

| Action | Description |
|---|---|
| **Edit caption** | Type or paste a caption; auto-saved |
| **Approve / Unapprove** | Only approved items are eligible to be posted |
| **AI regenerate** | Re-run AI vision analysis for a fresh caption |
| **Reorder** | Use up/down buttons to adjust publish order |
| **Override platforms** | Override the queue's platform selection for one item |
| **Crop / Edit image** | Crop the shown image (with the queue watermark drawn over it for reference; it is stamped at post time) or open it in the image editor |
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

### Quick Reel

Quick Reel turns a phone clip into an Instagram reel in about a minute, straight from the convention floor.

1. In the queue's settings, pick a **Quick Reel template**: a video template from the media library.
2. On the queue page, click **Quick Reel** and pick a clip. It uploads with a progress bar.
3. Drag the two handles on the filmstrip to choose where the reel cuts into the clip and out of it (3–90 s; arrow keys nudge, Shift for a second). The kept part sets the reel's length; the template's animations play over its own length and its music plays on.
4. Add a few words of hints. The reel renders in the browser while the AI writes the caption (the team's "Rewrite for Instagram" prompt plus the queue's hashtags).
5. Review the reel and edit the caption, then **Publish**: it is scheduled on the queue's Instagram account and footer 30 minutes after the clip was uploaded, or right away once that has passed. **Save as draft** keeps it as a draft to finish in the post editor instead.

The clip goes into the template's largest clip frame, else into a large picture frame, else full frame under everything.

---

## Instagram Growth

Tools for growing an Instagram account towards a follower goal (teams only). The Instagram login asks for Insights access; accounts connected before that need a reconnect.

- **Growth Settings** (team admin, **Administration → Growth Settings**): the follower goal (target, start and end, a halfway review), the **weekly plan** (recurring slots by weekday, time and format: reel, carousel, image, story or any), the single-image warning limit, the **Rewrite for Instagram** prompt and the limit on open agent drafts.
- **Growth** page (**Content → Growth**): the goal tracker (weekly net followers vs. required, halfway status), a weekly report (top posts by shares and saves, sends and saves per reach by format) and ad candidates (reels and carousels in the top 20% of the last 30 days). Follower counts are recorded daily and post Insights read regularly; **Refresh** reads them now.
- **Calendar**: open weekly-plan slots show as dashed placeholders (click one to start a post of that type at that time), each week shows its format mix, and the views show ISO week numbers.
- **Post editor**: Instagram collaborators (up to 3, suggested from BGG publishers) and image tags, a caption checker (hook, closing question, share prompt, 3–5 hashtags, a tagged account) and **Rewrite for Instagram**. Published posts show their Insights. The News and Episode forms have the same caption checker.

---

## Agents

**Post Tools → Agents** runs AI agents inside SocialPod: a prompt on a schedule (set times, hourly, every N minutes/hours/days, or manual), executed by an OpenRouter model you pay for, with the same tools as the MCP server plus web search. Agents fill the draft queue; nothing is published without a person. Set your OpenRouter key and default model on the **Profile** page. See [[Content Agent]] for the setup, the tools and a sample run.

---

## MCP Server and Connectors

SocialPod is an MCP server at `/api/mcp`. Point Claude Code, OpenCode or another MCP client at it with your API token as a bearer token, or add it as a custom connector in Claude.ai or ChatGPT, which sign in with OAuth (requires `APP_URL` to be the public HTTPS URL). Connected apps are listed, and can be disconnected, on the Profile page.

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
- Each user's posts and footers are isolated by default.
- **Teams**: admins can create teams and assign users. Team members share posts and footers scoped to the team.

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

Saved entries are kept on the **Drafts** tab; publishing moves a draft to the **Posted** tab (cleared with **Remove All**). The Episodes page works the same way.

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

> **Currently disabled.** The integration is switched off in the frontend (`ADOBE_EXPRESS_ENABLED` in `frontend/src/features.ts`): the "Create Design" buttons and the settings card are hidden and the SDK is never loaded. A stored Client ID is kept. Set the flag to `true` to turn it back on.

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
