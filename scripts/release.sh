#!/usr/bin/env bash
# Release bookkeeping script.
#
# Usage:
#   scripts/release.sh <tag> <summary>
#
# Example:
#   scripts/release.sh v0.1.0 "Initial release with up/down/status/ssh"
#
# What it does:
#   1. Validates tag format and clean working tree
#   2. Inserts changelog entry into CHANGELOG.md
#   3. Commits bookkeeping changes
#   4. Creates an annotated git tag
#   5. Prints the push command
#
# After running, review with "git log --oneline -2" and "git diff HEAD~1",
# then push: git push && git push origin <tag>

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHANGELOG_MD="${REPO_ROOT}/CHANGELOG.md"

# --- Argument parsing ---

if [[ $# -lt 2 ]]; then
    echo "Usage: scripts/release.sh <tag> <summary>"
    echo "Example: scripts/release.sh v0.1.0 \"Initial release with up/down/status/ssh\""
    exit 1
fi

TAG="$1"
shift
SUMMARY="$*"

# Validate tag format (v followed by semver).
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
    echo "Error: tag must be in format v<major>.<minor>.<patch>[-suffix] (e.g. v0.1.0), got: ${TAG}"
    exit 1
fi

echo "==> Tag: ${TAG}"
echo "==> Summary: ${SUMMARY}"
echo ""

# --- Check for uncommitted changes ---

if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: working tree is dirty. Commit or stash changes first."
    git status --short
    exit 1
fi

# --- Check tag doesn't already exist ---

if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Error: tag ${TAG} already exists."
    exit 1
fi

# --- Update changelog ---

echo "==> Updating CHANGELOG.md..."

TODAY=$(date +%Y-%m-%d)

# Create CHANGELOG.md if it doesn't exist.
if [[ ! -f "$CHANGELOG_MD" ]]; then
    cat > "$CHANGELOG_MD" << 'HEADER_EOF'
# Changelog

All notable releases are documented here.

---

HEADER_EOF
fi

# Build the new entry as a temp file.
TEMP_ENTRY=$(mktemp)
cat > "$TEMP_ENTRY" << ENTRY_EOF

## ${TAG}

**${TODAY}** — ${SUMMARY}

---
ENTRY_EOF

# Insert after the first "---" line (after the header).
# Use awk instead of sed -i for cross-platform compatibility (macOS + Linux).
TEMP_OUTPUT=$(mktemp)
awk -v entry_file="$TEMP_ENTRY" '
    /^---$/ && !done {
        print
        while ((getline line < entry_file) > 0) print line
        close(entry_file)
        done = 1
        next
    }
    { print }
' "$CHANGELOG_MD" > "$TEMP_OUTPUT"

mv "$TEMP_OUTPUT" "$CHANGELOG_MD"
rm -f "$TEMP_ENTRY"

# --- Commit ---

echo "==> Committing bookkeeping..."

git add "$CHANGELOG_MD"
git commit -m "Release bookkeeping for ${TAG}

${SUMMARY}"

# --- Tag ---

echo "==> Creating tag ${TAG}..."

git tag -a "$TAG" -m "${TAG}: ${SUMMARY}"

# --- Done ---

echo ""
echo "============================================"
echo "  Release ${TAG} ready!"
echo ""
echo "  Summary: ${SUMMARY}"
echo "  Commit:  $(git rev-parse --short HEAD)"
echo "  Tag:     ${TAG}"
echo ""
echo "  Review:"
echo "    git log --oneline -2"
echo "    git diff HEAD~1"
echo ""
echo "  Push:"
echo "    git push && git push origin ${TAG}"
echo "============================================"
