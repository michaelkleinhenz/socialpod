# Content Agent

SocialPod's MCP server (`/api/mcp`) has everything an AI agent needs to run without supervision: it can find content on the web, check that a story hasn't been covered yet, import games from BoardGameGeek, build post images from the team's watermarks and media library, and put finished drafts into the Drafts list for people to review. Nothing is published without a person: the agent creates drafts, and people schedule them.

## Built-in agents

You don't need an outside MCP client to run an agent: **Post Tools → Agents** runs them inside SocialPod.

1. On the **Profile** page, under *Agents*, enter your [OpenRouter](https://openrouter.ai/keys) API key and pick a default model. The model must support tool calling; the picker lists OpenRouter's tool-calling models with their prices. Usage is billed to your OpenRouter account.
2. On the **Agents** page, write a **global prompt** (house rules every agent follows: language, tone, what never to post) and create one or more agents, each with a name, a prompt and a schedule: at set times of day (optionally on chosen weekdays), every hour at a given minute, every N minutes/hours/days (at least 15 minutes), or manual runs only. Times are read in the agent's timezone. An agent can override the model and the number of model turns per run (default 30, max 100).
3. Each run gets the same tools as the MCP server (under the same names, with your rights, never admin rights) plus `web_search` (a web search through OpenRouter's web plugin, with the agent's own model) and `fetch_url` (a page's text, title, description, image and date). A prompt written for an MCP client therefore works unchanged; the "Insert example" button in the agent form fills in one for the news use case.
4. **Run now** starts an agent outside its schedule; **Runs** shows the last runs with every tool call, its arguments and result, the model's summary, tokens and cost, and follows a run in progress. Runs are kept for 30 days. **Stop** cancels a run.

What a built-in agent creates carries the origin `agent-<name>` (e.g. `agent-news-scout`) unless its prompt passes another one, and counts towards the draft limit below.

## Setup

1. **Connect the agent.** (Not needed for built-in agents.) Point any MCP client at `https://<your-host>/api/mcp`. For an agent that runs unattended (a scheduled Claude routine, a cron job), use a bearer token from the Profile page rather than OAuth: OAuth access tokens expire after an hour. A team token (`st_...`) works too.
2. **Describe your watermarks.** In the Media Library (Watermarks filter), give each watermark a description of what it looks like and how it is meant to be used, e.g. "round logo, place in a corner at 15% width" or "full-size frame with the news banner at the bottom". `list_watermarks` also reports what the team uses each one for (BGG overlay, news overlay, ...) and whether it is a full-canvas overlay.
3. **Fill the media library.** Upload photos, box art, backgrounds and logos on the Media Library page, with a description and tags. The agent searches them with `list_media` and builds images from them. Fonts (.ttf or .otf) go there too: the agent finds them with `list_media` (`kind: "font"`, `preview: true` shows a sample of each) and sets text in them. Add the team colours with "Add Colour", one entry per colour, and describe in each how to use it ("headlines and badges", "never behind photos"); the agent lists them with `kind: "color"` and puts the hex values into `compose_image`.
4. **Set a draft limit.** On Growth settings, "Content agents" caps how many agent drafts may wait for review at once. Once it is reached, creating more drafts fails until people have scheduled or deleted some.
5. **Plan the week.** The weekly plan on Growth settings tells the agent which formats to make and when; `get_open_slots` lists the slots nothing fills yet.

## Tools at a glance

| Need | Tools |
|---|---|
| What to make, when | `get_growth_settings`, `get_open_slots`, `get_account_insights`, `get_post_insights` |
| Avoid duplicates and floods | `find_existing`, `get_draft_queue` |
| Content from the web | `capture_url` (page to draft, in house style), `import_bgg` (BGG game with processed cover, text, hashtags, collaborators) |
| Images | `list_watermarks`, `list_media`, `get_image`, `upload_from_url`, `compose_image`, `add_media` |
| Text | `rewrite_instagram`, `check_instagram_caption`, `list_mentions` |
| Drafts | `create_post` (with `status: "draft"`, `sourceUrl`, `origin`), `update_post`, `create_news_draft`, `create_episode_draft` |

Everything an agent creates carries an `origin` (default `mcp`; pass your own short label such as `agent`), so the Calendar's Drafts tab marks those drafts and can filter by origin. Pass `sourceUrl` on `create_post` so `find_existing` recognises the story next time: URLs are compared normalised (BGG games by ID, other pages without tracking parameters, `www.` or trailing slash).

### compose_image

Builds an image on the server and returns its URL plus a preview the agent can look at:

```json
{
  "base": {"mediaId": "66f1..."},
  "format": "4:5",
  "fit": "cover",
  "filter": "vivid",
  "layers": [
    {"type": "gradient", "anchor": "bottom", "height": 0.45},
    {"type": "text", "text": "\"Wingspan\" bekommt eine Asien-Edition", "anchor": "bottom", "margin": 0.08, "fontSize": 0.07},
    {"type": "image", "watermarkId": "66e0...", "anchor": "full"}
  ]
}
```

Bases and image layers take `url` (an upload or a public image), `mediaId` or `watermarkId`. Geometry is given in fractions of the canvas. Formats: `1:1`, `4:5` (default), `9:16`, `16:9`, `1.91:1`, `original`. Fits: `cover` (crop, with `focusX`/`focusY`), `contain`, `blur` (letterbox on a blurred copy, good for box art). Text can use the built-in fonts or a font from the media library (`"fontMediaId": "<media id>"`), with outline, shadow or a background box.

#### Templates as the base

Designs saved as templates in the media library ("New Template", built in the image editor) can be the base of `compose_image`. That keeps a series, such as the slides of a carousel, in the team's design: the agent only swaps the texts and images. `list_media` with `kind: "template"` lists each template's layers as `templateLayers` (index, kind, the text or image, the position as fractions of the canvas); changes name a layer by its index:

```json
{
  "template": {
    "mediaId": "6ace50c9...",
    "layers": [
      {"index": 3, "text": "\"Wingspan\" bekommt\neine Asien-Edition"},
      {"index": 4, "text": "Seite 2/7", "color": "#a31b22"},
      {"index": 1, "url": "/api/uploads/cover.jpg", "focusY": 0.3},
      {"index": 2, "hide": true}
    ]
  }
}
```

The template sets the canvas (`format`, `width` and `height` are ignored). A new text keeps the layer's font, size, colour and alignment, and `fontScale` shrinks it for a longer one; lines only break at `\n`. A swapped image is cropped to fill the layer's frame, keeping its corners and border. A `base` goes under the template's layers and extra `layers` on top. Fonts from the media library are used as in the editor; system fonts the editor offers (Arial and the like) are drawn in the built-in fonts on the server, so templates meant for agents should use library fonts.

## A run, step by step

A prompt along these lines works well for a scheduled agent:

> You fill SocialPod's draft queue for our board game account. Each run:
> 1. Call `get_draft_queue`. If `remaining` is 0, stop.
> 2. Call `get_open_slots` (next 7 days) and `get_account_insights` to see which formats are needed and what performed well.
> 3. Search the web for board game news from the last two days (publisher announcements, crowdfunding launches, awards, German releases).
> 4. For each candidate, call `find_existing` with its URL and the game title. Skip anything found.
> 5. For a game with a BGG page, call `import_bgg` with `createDraft: true`, `scheduledAt` set to an open slot's `at`, and `origin: "agent"`. Otherwise call `capture_url` with `entityType: "post"`, or write the post yourself.
> 6. For the image: look at `list_watermarks` (with `preview: true` the first time) and `list_media`; build the image with `compose_image`, check the preview, adjust once if needed, and set it with `update_post` (`imageUrls`).
> 7. Write the Instagram caption with `rewrite_instagram`, check it with `check_instagram_caption` and fix what is missing. Game titles go in plain double quotes; no em dashes.
> 8. Stop after three drafts, or when `get_draft_queue` says the queue is full.

People then review the drafts in the Calendar's Drafts tab, edit what they like and schedule them.
