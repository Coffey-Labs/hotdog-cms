#!/usr/bin/env bash
# cla-check.sh [BASE [HEAD]]: before merging a pull request, list the commit
# authors between BASE (default origin/main) and HEAD (default HEAD) who
# aren't in CONTRIBUTORS.md as HEAD has it, so haven't accepted CLA.md.
# Exits 1 if there are any.
set -euo pipefail
base=${1:-origin/main}
head=${2:-HEAD}
signed=$(git show "$head:CONTRIBUTORS.md" | grep -oiE '[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}' | tr 'A-Z' 'a-z' | sort -u)
missing=0
while IFS=$'\t' read -r name email; do
	lower=$(printf '%s' "$email" | tr 'A-Z' 'a-z')
	if ! grep -qxF "$lower" <<<"$signed"; then
		echo "not in CONTRIBUTORS.md: $name <$email>"
		missing=1
	fi
done < <(git log --format='%an%x09%ae' "$base..$head" | sort -u)
if [ "$missing" = 0 ]; then
	echo "every author between $base and $head has accepted the contributor agreement"
fi
exit "$missing"
