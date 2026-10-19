#!/usr/bin/env sh
# Request an Omarchy marketplace listing update for the current HEAD of
# this repository's default branch. Called by `make marketplace-update`;
# files a verify-plugin issue in omacom/omarchy-plugin-marketplace under
# the maintainer's own `gh` login, asking it to pick up that commit.
#
# The marketplace only accepts a target that is the default branch's
# current HEAD, so this script resolves that HEAD from GitHub rather than
# any local tag or checkout. It then refuses unless, at that HEAD, the
# plugin is still pinned to the latest release: manifest.json and
# helper-version must equal the release tag's version, and the release
# must have its assets. Both files are read from GitHub at that HEAD, not
# from this checkout, since a local worktree may not be at HEAD. It lists
# every commit between the release tag and HEAD, so the maintainer filing
# the issue can see what they are attesting to, then builds the issue body
# from the marketplace repository's own verify-plugin.yml issue form: the
# form is fetched live and parsed for its headings and checkbox wording,
# so a change to that template upstream is never silently missed. It
# prints the title and body and asks before filing anything.
#
# DRY_RUN=1 prints the commit list, title and body and exits without
# asking or filing.
set -eu

source_repo=timlittle/omamessenger
repository_url="https://github.com/$source_repo"
marketplace_repo=omacom/omarchy-plugin-marketplace
plugin_id=io.github.omamessenger
action_text="Verify and publish a newer upstream commit"

for tool in gh jq base64; do
    command -v "$tool" >/dev/null 2>&1 || { printf 'marketplace-update: %s is required\n' "$tool" >&2; exit 1; }
done

tag=$(gh release view --json tagName --jq .tagName)
assets=$(gh release view "$tag" --json assets --jq '.assets | length')
[ "$assets" -gt 0 ] || { printf 'marketplace-update: release %s has no assets yet\n' "$tag" >&2; exit 1; }

head_sha=$(gh api "repos/$source_repo/commits/main" --jq .sha)
case "$head_sha" in
    *[!0-9a-fA-F]*) head_sha='' ;;
esac
[ "${#head_sha}" -eq 40 ] || { printf 'marketplace-update: could not resolve the default branch HEAD\n' >&2; exit 1; }

manifest_version=$(gh api "repos/$source_repo/contents/manifest.json?ref=$head_sha" --jq .content | base64 -d | jq -r .version)
helper_version=$(gh api "repos/$source_repo/contents/helper-version?ref=$head_sha" --jq .content | base64 -d | cut -d'#' -f1 | tr -d ' \n')

tag_version=${tag#v}
if [ "$tag_version" != "$manifest_version" ] || [ "$tag_version" != "$helper_version" ]; then
    printf 'marketplace-update: HEAD %s is not pinned to the latest release %s (manifest.json=%s, helper-version=%s); the existing validation and baseline do not cover that newer commit\n' \
        "$head_sha" "$tag" "$manifest_version" "$helper_version" >&2
    exit 1
fi

commits=$(gh api "repos/$source_repo/compare/$tag...$head_sha" --jq '.commits[] | (.sha[0:7] + " " + (.commit.message | split("\n")[0]))')

template=$(gh api "repos/$marketplace_repo/contents/.github/ISSUE_TEMPLATE/verify-plugin.yml" --jq .content | base64 -d)

# Each "id:" line starts a new form field; the first "label:" after it is
# that field's heading, and any "- label:" lines after that are its
# checkbox options. Keyed on the field id, not position, so the fields
# below can be pulled out regardless of what order the form lists them in.
parsed=$(printf '%s\n' "$template" | awk '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    /^[ \t]*id:[ \t]*/ {
        line = $0
        sub(/^[ \t]*id:[ \t]*/, "", line)
        cur = trim(line)
        have_label = 0
        next
    }
    /^[ \t]*-[ \t]*label:[ \t]*/ {
        line = $0
        sub(/^[ \t]*-[ \t]*label:[ \t]*/, "", line)
        print "CHECKBOX\t" cur "\t" trim(line)
        next
    }
    /^[ \t]*label:[ \t]*/ {
        if (have_label) next
        line = $0
        sub(/^[ \t]*label:[ \t]*/, "", line)
        print "LABEL\t" cur "\t" trim(line)
        have_label = 1
        next
    }
')

# field_label prints the heading recorded for a form field id.
field_label() {
    printf '%s\n' "$parsed" | awk -F'\t' -v id="$1" '$1 == "LABEL" && $2 == id { print $3; exit }'
}

# checkbox_text prints the checkbox wording recorded for a form field id.
checkbox_text() {
    printf '%s\n' "$parsed" | awk -F'\t' -v id="$1" '$1 == "CHECKBOX" && $2 == id { print $3; exit }'
}

action_label=$(field_label verification-action)
plugin_label=$(field_label plugin-id)
repo_label=$(field_label repository)
commit_label=$(field_label target-commit)
ack_label=$(field_label acknowledgment)
std_label=$(field_label standard-installation-acknowledgment)
ack_text=$(checkbox_text acknowledgment)
std_text=$(checkbox_text standard-installation-acknowledgment)

for value in "$action_label" "$plugin_label" "$repo_label" "$commit_label" "$ack_label" "$std_label" "$ack_text" "$std_text"; do
    [ -n "$value" ] || { printf 'marketplace-update: could not read verify-plugin.yml (template changed?)\n' >&2; exit 1; }
done

title="[Verify]: OmaMessenger $tag"

body_file=$(mktemp)
trap 'rm -f "$body_file"' EXIT
cat >"$body_file" <<EOF
### $action_label

$action_text

### $plugin_label

$plugin_id

### $repo_label

$repository_url

### $commit_label

$head_sha

### $ack_label

- [x] $ack_text

### $std_label

- [x] $std_text
EOF

printf 'Commits between %s and HEAD (%s):\n' "$tag" "$head_sha"
if [ -n "$commits" ]; then
    printf '%s\n' "$commits"
else
    printf '  (none)\n'
fi
printf '\n%s\n\n' "$title"
cat "$body_file"

[ "${DRY_RUN:-0}" != "1" ] || exit 0

printf 'File this issue? [y/N] '
IFS= read -r answer || answer=''
if [ "$answer" != "y" ]; then
    printf 'Not filed.\n'
    exit 0
fi

gh issue create --repo "$marketplace_repo" --title "$title" --body-file "$body_file"
