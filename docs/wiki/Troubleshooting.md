# Troubleshooting

## Installation and Startup

**Container won't start — MongoDB connection refused**
- MongoDB needs to pass its health check first. Wait a few seconds and run `make logs`.

**First user is not admin**
- The admin flag is set during registration when the users collection is empty. To reset, clear the `users` collection in MongoDB.

**Watermark images show as broken after a container restart**
- Uploaded files are stored in `UPLOAD_DIR`. The provided `docker-compose.yml` mounts a named volume (`uploads_data`) at that path. If you deployed before this volume was added, run `make down && make up` — Docker will create the volume.

**Watermark images broken immediately after upload**
- Ensure `UPLOAD_DIR` matches the volume mount path. The default setup sets `UPLOAD_DIR=/app/uploads` and mounts `uploads_data:/app/uploads`.

---

## Instagram

**Instagram OAuth fails with "redirect URI mismatch"**
- Ensure `APP_URL` in SocialPod settings exactly matches the redirect URI configured in your Meta app (including protocol and no trailing slash).

**Images not showing in Instagram posts**
- Instagram fetches images from your server. `APP_URL` must be publicly accessible, not `localhost`.

**Instagram feed is empty**
- Confirm your Instagram account is connected in **Accounts** and has published posts.
- Try reconnecting the account if posts are not appearing.

---

## Bluesky

**Bluesky posts fail with "auth failed"**
- Verify the handle and app password are correct.
- If using a custom PDS, ensure the PDS host URL is correct and reachable.

---

## X (Twitter)

**Posts fail with "401 Unauthorized"**
- Verify all four OAuth 1.0a credentials are correct and were generated for the same Twitter app.
- Ensure the app has **Read and Write** permissions. Tokens generated before upgrading permissions must be regenerated.

**Posts fail with "403 Forbidden / oauth1 app permissions"**
- Your app's permissions are set to **Read only**. Change to **Read and Write** in the X Developer Portal → your app → **Settings → User authentication settings**.
- After changing permissions you must **regenerate** your Access Token and Secret — delete the account in SocialPod and re-add it.

**Media upload fails**
- Only JPEG, PNG, GIF, and WebP are supported. Videos are not currently supported.
- The Twitter v1.1 media endpoint enforces a 5 MB file size limit for images.

---

## Mastodon

**Posts fail with "403 Forbidden"**
- Ensure the access token has the `write:statuses` and `write:media` scopes. Regenerate the token in your Mastodon instance settings.

**Account shows wrong instance handle**
- The instance field must be the hostname only (e.g. `mastodon.social`), without a leading `@` or trailing slash.

---

## AI Features

**AI text generation button not appearing**
- The wand button only appears when an OpenRouter API key is saved in **Settings**. Ensure the key is valid and the model is set.

**Convention mode AI analysis returns no captions / errors**
- Confirm an OpenRouter API key and a vision-capable model are configured in **Settings → AI / OpenRouter**.
- Not all models support image inputs — use a model such as `google/gemini-flash-1.5` or `anthropic/claude-3-haiku`.
- Items with analysis errors display the error message inline; click the AI icon to retry.

**Convention queue shows "more items than slots" warning**
- Either reduce the number of approved items, extend the **End date**, increase **Posts per day**, or reduce **Min. hours between posts**.

---

## n8n

**n8n node not appearing after installation**
- Confirm the node directory contains a `dist/` folder with compiled `.js` files (included out of the box).
- Make sure you ran `npm install --omit=dev`, not `npm install`.
- Restart n8n completely (not just reload).
- Check n8n logs for import errors related to `n8n-nodes-socialpod`.

**n8n node install fails with `isolated-vm` / `node-gyp` errors**
- You ran `npm install` without `--omit=dev`. Use `npm install --omit=dev` — n8n provides `n8n-workflow` at runtime, so the build-time dependency is not needed.

**n8n node shows fewer operations than expected**
- The n8n node supports 21 operations across 6 resources: Post (7 ops), Mention (6 ops), Suffix (4 ops), Watermark (2 ops), Account (1 op), and AI Text (1 op). If operations are missing, update to the latest version of `n8n-nodes-socialpod`.

---

## Team Features

**News or Episode Creator tabs not appearing**
- The `news_creator` and `episode_creator` plugins must be enabled per team by an admin at **Admin → Teams → Plugins**.
- Each plugin requires a valid n8n webhook URL and bearer token in **Admin → Teams → Team Settings**.

**Team invites link expired or invalid**
- Invite tokens expire after 7 days. Generate a new invite from **Admin → Teams → Manage Members**.
- The invite page is public at `/invite?token=...` and does not require authentication.

**BGG import returns errors**
- BoardGameGeek's XML API sometimes returns `202 Accepted` when data is being generated. The backend retries up to 5 times with 2-second delays.
- For convention BGG item imports, each URL is processed independently — check the per-URL error messages in the response.

**Publisher handle not resolving during BGG import**
- Check **Admin → Publisher Handles** — if the name isn't in the local catalog, the system falls back to AI handle lookup.
- AI handle lookup requires an OpenRouter API key with a capable model.
