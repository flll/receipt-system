#!/bin/bash -e

TEMP_FILE="security.log"

echo "タグの監視を開始します..."

get_latest_sha_tag() {
    curl -s "https://hub.docker.com/v2/repositories/fjlli/receipt-system/tags/" | \
    jq -r '.results[].name' | \
    grep '^sha-' | \
    head -n 1
}

if [ ! -f "$TEMP_FILE" ]; then
    : > "$TEMP_FILE"
fi

while true; do
    current_tag=$(get_latest_sha_tag)
    previous_tag=$(cat "$TEMP_FILE")
    if [ "$current_tag" != "$previous_tag" ] && [ -n "$current_tag" ]; then
        echo "fjlli/receipt-system:${current_tag}"
        echo "$current_tag" > "$TEMP_FILE"
    fi
    sleep 3
done