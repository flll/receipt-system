#!/bin/bash -e

TEMP_FILE="security.log"

echo "タグの監視を開始します..."

# ✷ latestタグのManifest digestを取得
get_latest_digest() {
    curl -s "https://hub.docker.com/v2/repositories/fjlli/receipt-system/tags/latest" | \
    jq -r '.digest'
}

if [ ! -f "$TEMP_FILE" ]; then
    : > "$TEMP_FILE"
fi

while true; do
    current_digest=$(get_latest_digest)
    previous_digest=$(cat "$TEMP_FILE")
    if [ "$current_digest" != "$previous_digest" ] && [ -n "$current_digest" ] && [ "$current_digest" != "null" ]; then
        echo "$(date '+%Y-%m-%d %H:%M:%S') fjlli/receipt-system@${current_digest}"
        echo "$current_digest" > "$TEMP_FILE"
        echo "================================================"
    fi
    sleep 5
done