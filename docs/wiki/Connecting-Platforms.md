# Connecting Platforms

SocialPod supports six social networks. Each requires its own credentials, generated from the platform's developer portal.

---

## Bluesky

Bluesky uses **app passwords** — no OAuth flow required.

### 1. Generate an App Password

1. Log in to [bsky.app](https://bsky.app).
2. Go to **Settings → Privacy and Security → App passwords**.
3. Click **Add App Password**, give it a name (e.g. "SocialPod"), and click **Create**.
4. Copy the generated password (format: `xxxx-xxxx-xxxx-xxxx`). You will not be able to see it again.

### 2. Add the Account in SocialPod

1. Navigate to **Accounts** in the sidebar.
2. Click **Add Bluesky** and enter:
   - **Handle** — your Bluesky handle (e.g. `yourname.bsky.social`)
   - **App Password** — the password from step 1
   - **PDS Host** (optional) — leave blank for `https://bsky.social`; only change for a custom PDS
3. Click **Add Account**.

### Post Capabilities

- Text posts up to 300 characters
- Up to 4 images per post
- Automatic hashtag detection and richtext facets
- Custom PDS support

---

## Instagram

SocialPod uses the **Instagram Business/Standalone** OAuth flow via the Meta Developers platform.

### 1. Create a Meta App

1. Go to [developers.facebook.com](https://developers.facebook.com/) and log in.
2. Click **My Apps → Create App → Other → Consumer**.
3. Name your app and click **Create**.
4. In the app dashboard, find **Instagram** and click **Set Up**.

### 2. Configure the OAuth Redirect URI

1. In your Meta app, go to **Instagram → Basic Display**.
2. Under **Valid OAuth Redirect URIs**, add:
   ```
   https://your-domain.com/api/auth/instagram/callback
   ```
   > Instagram requires HTTPS — use ngrok or a similar tool for local testing.
3. Save your changes.

### 3. Note Your App Credentials

From **Instagram → Basic Display**:
- **Instagram App ID**
- **Instagram App Secret** (click "Show" to reveal)

### 4. Configure SocialPod

1. Go to **Settings** in the sidebar.
2. Enter the **Application URL**, **Instagram App ID**, and **Instagram App Secret**.
3. Click **Save Settings**.

### 5. Connect an Instagram Account

1. Go to **Accounts → Connect Instagram**.
2. Authorize SocialPod on the Instagram OAuth page.
3. The account appears in the accounts list.

### Post Types

| Post Type | Media Required | Description |
|---|---|---|
| `post` (default) | 1–10 images | Regular feed post or carousel |
| `story` | 1 image or video | Instagram Story (disappears after 24h) |
| `reel` | 1 video (MP4) | Instagram Reel |

Additional requirements:
- Instagram requires at least one media file per post — text-only posts are not supported.
- Captions up to 2,200 characters (not applicable to Stories).
- Media files must be publicly accessible via `APP_URL`.
- The access token expires after ~60 days and must be reconnected.

### Scopes Requested

- `instagram_business_basic` — read profile info
- `instagram_business_content_publish` — create and publish posts
- `instagram_business_manage_messages` — manage messages

---

## X (Twitter)

> **X API access is not free.** You need at least the **Basic** plan (~$100/month) to post via the API. Check [developer.twitter.com](https://developer.twitter.com/en/products/twitter-api) for current pricing.

SocialPod uses **OAuth 1.0a** with user-context credentials.

### 1. Create a Twitter Developer App

1. Go to [developer.twitter.com](https://developer.twitter.com/) and subscribe to a paid plan.
2. Create a new project and app.
3. Go to **Settings → User authentication settings**.
4. Set **App permissions** to **Read and Write** (required for posting).
5. Save the settings.

### 2. Gather Your Credentials

From **Keys and Tokens**, click **Generate** next to **Access Token and Secret**.

> Always generate tokens *after* setting Read and Write permissions. Tokens issued under Read-only cannot post even after the app is upgraded.

| Credential | Location |
|---|---|
| **API Key** (Consumer Key) | "Consumer Keys" section |
| **API Key Secret** | "Consumer Keys" section |
| **Access Token** | "Authentication Tokens" section |
| **Access Token Secret** | "Authentication Tokens" section |

### 3. Add the Account in SocialPod

1. Navigate to **Accounts → Add X/Twitter**.
2. Enter the four credentials.
3. Click **Add Account**.

### Post Capabilities

- Text up to 280 characters
- Up to 4 images per tweet (JPEG, PNG, GIF, WebP; max 5 MB each)
- Per-platform text customization and suffix support

---

## Mastodon

Mastodon uses **Bearer tokens** generated directly from your instance — no developer portal required.

### 1. Generate an Access Token

1. Log in to your Mastodon instance.
2. Go to **Preferences → Development → New Application**.
3. Give it a name and check these scopes: `read`, `write:statuses`, `write:media`.
4. Click **Submit**, open the app, and copy **Your access token**.

### 2. Add the Account in SocialPod

1. Navigate to **Accounts → Add Mastodon** and enter:
   - **Instance** — hostname only (e.g. `mastodon.social`), no leading `@` or trailing slash
   - **Access Token** — the token from step 1
2. Click **Add Account**.

### Post Capabilities

- Text up to 500 characters
- Up to 4 images (uploaded via the v2 media endpoint with async polling)
- Fediverse handle displayed as `@user@instance`
- Per-platform text customization and suffix support

---

## Threads

Threads uses the **Meta Threads API** with a long-lived user access token.

### 1. Create a Threads App on Meta

1. Go to [developers.facebook.com](https://developers.facebook.com/) and create an app.
2. Select **Other → Consumer**, name it, and click **Create**.
3. Find **Threads API** in the product list and click **Set Up**.

### 2. Get a Long-Lived Access Token

1. Under **Threads API → User Token Generator**, add your Threads account as a test user.
2. Generate a short-lived token with scopes `threads_basic` and `threads_content_publish`.
3. Exchange it for a long-lived token via the Threads API token exchange endpoint (valid for 60 days, refreshable).

> Once your app is reviewed and approved by Meta, users can connect directly without being added as test users.

### 3. Add the Account in SocialPod

1. Navigate to **Accounts → Add Threads**.
2. Enter the long-lived access token.
3. Click **Add Account**.

### Post Capabilities

- Text posts up to 500 characters
- Single image posts (image must be publicly accessible via `APP_URL`)
- Carousel posts with up to 10 images
- Per-platform text customization and suffix support

---

## LinkedIn

LinkedIn uses **OAuth 2.0** for authentication.

### 1. Create a LinkedIn App

1. Go to [linkedin.com/developers](https://www.linkedin.com/developers/) and click **Create App**.
2. Fill in the required fields (app name, associated LinkedIn Page, logo).
3. Click **Create App**.

### 2. Configure App Permissions

1. Go to the **Products** tab and request access to **Share on LinkedIn** (grants `w_member_social` scope).
2. Wait for approval (usually instant for Share on LinkedIn).

### 3. Get an Access Token

1. In the **Auth** tab, add a redirect URL.
2. Use LinkedIn's OAuth 2.0 authorization flow to obtain a token with `w_member_social` scope.
   - The [OAuth 2.0 PKCE tool](https://www.linkedin.com/developers/tools/oauth) in the LinkedIn Developer Portal can generate a token quickly for testing.
3. The access token expires after 60 days.

### 4. Add the Account in SocialPod

1. Navigate to **Accounts → Add LinkedIn**.
2. Enter your OAuth 2.0 access token.
3. Click **Add Account**.

### Post Capabilities

- Text posts up to 3,000 characters
- Single or multiple image posts
- Posts published to member's feed with public visibility
- Per-platform text customization and suffix support
