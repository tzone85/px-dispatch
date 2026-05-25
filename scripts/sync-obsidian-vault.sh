#!/usr/bin/env bash
#
# sync-obsidian-vault.sh — keep docs/obsidian/ in sync with the Obsidian
# vault under iCloud.
#
# Usage:
#   scripts/sync-obsidian-vault.sh            # one-shot rsync
#   scripts/sync-obsidian-vault.sh --watch    # watch + sync on every change
#   scripts/sync-obsidian-vault.sh --dest /custom/path/px-dispatch
#   scripts/sync-obsidian-vault.sh --dry-run  # show what would change
#
# Defaults:
#   SOURCE = <repo>/docs/obsidian/
#   DEST   = $PX_DISPATCH_VAULT (env), else iCloud Obsidian Documents/px-dispatch
#
# Notes:
#   - rsync --delete keeps the vault in lock-step with the repo (vault
#     edits will be overwritten on the next sync — make changes in the
#     repo, not in Obsidian).
#   - --watch needs `fswatch` (`brew install fswatch`).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="$REPO_ROOT/docs/obsidian/"
DEFAULT_DEST="$HOME/Library/Mobile Documents/iCloud~md~obsidian/Documents/px-dispatch"
DEST="${PX_DISPATCH_VAULT:-$DEFAULT_DEST}"

WATCH=0
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --watch) WATCH=1; shift ;;
    --dest) DEST="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help)
      sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [[ ! -d "$SOURCE" ]]; then
  echo "source missing: $SOURCE" >&2
  exit 1
fi

mkdir -p "$DEST"

RSYNC_FLAGS=(-a --delete --exclude='.DS_Store' --exclude='.obsidian/')
if [[ $DRY_RUN -eq 1 ]]; then
  RSYNC_FLAGS+=(-nv)
fi

do_sync() {
  rsync "${RSYNC_FLAGS[@]}" "$SOURCE" "$DEST"
  echo "[$(date +%H:%M:%S)] synced → $DEST"
}

if [[ $WATCH -eq 1 ]]; then
  if ! command -v fswatch >/dev/null 2>&1; then
    echo "watch mode needs fswatch — \`brew install fswatch\`" >&2
    exit 1
  fi
  echo "watching $SOURCE → $DEST (Ctrl+C to stop)"
  do_sync
  fswatch -o "$SOURCE" | while read -r _; do do_sync; done
else
  do_sync
fi
