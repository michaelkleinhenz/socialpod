#!/usr/bin/env bash
# Quick test: hit BGG directly with the same headers the scraper now sends.
# Usage: ./test-bgg-scraper.sh [bgg-game-url]
# Default URL is Wingspan (game 266192).

URL="${1:-https://boardgamegeek.com/boardgame/266192/wingspan}"
GAME_ID=$(echo "$URL" | grep -oP '/boardgame[^/]*/\K\d+')

echo "=== Scraper headers test ==="
echo "URL: $URL"
echo ""

echo "--- Scrape (HTML page, same headers as scrapeBGGGame) ---"
curl -s -o /dev/null -w "HTTP %{http_code}  time=%{time_total}s\n" \
  -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7" \
  -H "Accept-Language: en-US,en;q=0.9" \
  -H "Cache-Control: max-age=0" \
  -H "Upgrade-Insecure-Requests: 1" \
  -H "Sec-Fetch-Dest: document" \
  -H "Sec-Fetch-Mode: navigate" \
  -H "Sec-Fetch-Site: none" \
  -H "Sec-Fetch-User: ?1" \
  -H 'sec-ch-ua: "Chromium";v="136", "Google Chrome";v="136", "Not.A/Brand";v="99"' \
  -H "sec-ch-ua-mobile: ?0" \
  -H 'sec-ch-ua-platform: "Windows"' \
  -H "Referer: https://boardgamegeek.com/" \
  "$URL"

echo ""
echo "--- XML API (same headers as fetchBGGItem) ---"
XML_URL="https://boardgamegeek.com/xmlapi2/thing?id=${GAME_ID}&stats=1"
echo "URL: $XML_URL"
curl -s -o /dev/null -w "HTTP %{http_code}  time=%{time_total}s\n" \
  -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8" \
  -H "Accept-Language: en-US,en;q=0.9" \
  -H 'sec-ch-ua: "Chromium";v="136", "Google Chrome";v="136", "Not.A/Brand";v="99"' \
  -H "sec-ch-ua-mobile: ?0" \
  -H 'sec-ch-ua-platform: "Windows"' \
  -H "Referer: https://boardgamegeek.com/" \
  "$XML_URL"

echo ""
echo "--- Old bot UA (what was there before, for comparison) ---"
curl -s -o /dev/null -w "HTTP %{http_code}  time=%{time_total}s\n" \
  -A "Mozilla/5.0 (compatible; SocialPod/1.0)" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8" \
  -H "Accept-Language: en-US,en;q=0.5" \
  -H "Referer: https://boardgamegeek.com/" \
  "$URL"
