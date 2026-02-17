#!/bin/bash
set -e

# Syncs .env.production to Cloud Run focus-tube service.
# Uses --update-env-vars (safe, never wipes existing vars).
# Shows a diff of what will change before applying.

SERVICE="focus-tube"
REGION="us-central1"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env.production"

if [ ! -f "$ENV_FILE" ]; then
  echo "Error: $ENV_FILE not found"
  exit 1
fi

# Parse local env file into associative array
declare -A LOCAL_VARS
while IFS= read -r line; do
  line=$(echo "$line" | xargs)
  [[ -z "$line" || "$line" == \#* ]] && continue
  key="${line%%=*}"
  value="${line#*=}"
  LOCAL_VARS["$key"]="$value"
done < "$ENV_FILE"

# Fetch current Cloud Run env vars
echo "Fetching current env vars from $SERVICE..."
REMOTE_RAW=$(gcloud run services describe "$SERVICE" --region "$REGION" \
  --format="yaml(spec.template.spec.containers[0].env)" 2>/dev/null)

declare -A REMOTE_VARS
while IFS= read -r line; do
  if [[ "$line" =~ "name:" ]]; then
    key=$(echo "$line" | sed 's/.*name: //')
  elif [[ "$line" =~ "value:" ]]; then
    value=$(echo "$line" | sed 's/.*value: //')
    REMOTE_VARS["$key"]="$value"
  fi
done <<< "$REMOTE_RAW"

# Compare and show status
echo ""
echo "=== Env Var Status ==="
UPDATES=""
HAS_CHANGES=false

# Check all local vars
for key in $(echo "${!LOCAL_VARS[@]}" | tr ' ' '\n' | sort); do
  local_val="${LOCAL_VARS[$key]}"
  remote_val="${REMOTE_VARS[$key]}"
  masked="${local_val:0:4}***"

  if [ -z "$remote_val" ]; then
    echo "  + $key=$masked  (NEW)"
    HAS_CHANGES=true
  elif [ "$local_val" != "$remote_val" ]; then
    echo "  ~ $key=$masked  (CHANGED)"
    HAS_CHANGES=true
  else
    echo "  ✓ $key=$masked"
  fi
done

# Check for remote-only vars (not in local file)
for key in $(echo "${!REMOTE_VARS[@]}" | tr ' ' '\n' | sort); do
  if [ -z "${LOCAL_VARS[$key]+x}" ]; then
    masked="${REMOTE_VARS[$key]:0:4}***"
    echo "  ? $key=$masked  (REMOTE ONLY — not in .env.production)"
  fi
done

echo ""

if [ "$HAS_CHANGES" = false ]; then
  echo "Everything in sync. Nothing to do."
  exit 0
fi

# Build update string (only changed/new vars)
for key in "${!LOCAL_VARS[@]}"; do
  local_val="${LOCAL_VARS[$key]}"
  remote_val="${REMOTE_VARS[$key]}"
  if [ "$local_val" != "$remote_val" ]; then
    if [ -n "$UPDATES" ]; then
      UPDATES="$UPDATES,$key=$local_val"
    else
      UPDATES="$key=$local_val"
    fi
  fi
done

read -p "Apply changes? [y/N] " confirm
if [[ "$confirm" != [yY] ]]; then
  echo "Aborted."
  exit 0
fi

gcloud run services update "$SERVICE" --region "$REGION" --update-env-vars "$UPDATES"

echo ""
echo "Done! Env vars updated."
