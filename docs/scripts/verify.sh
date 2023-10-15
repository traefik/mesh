#!/bin/sh

PATH_TO_SITE="${1:-/app/site}"

set -eu

[ -d "${PATH_TO_SITE}" ]

NUMBER_OF_CPUS="$(grep -c processor /proc/cpuinfo)"

echo "=== Checking HTML content..."

# Search for all HTML files except the theme's partials
# and pipe this to htmlproofer with parallel threads
# (one htmlproofer per vCPU)
find "${PATH_TO_SITE}" -type f -not -path "/app/site/theme/*" \
    -name "*.html" -print0 \
| xargs -0 -r -P "${NUMBER_OF_CPUS}" -I '{}' \
  htmlproofer \
  --no-check_external_hash \
  --allow_missing_href \
  --ignore_missing_alt \
  --ignore_status_codes="0,500,501,503" \
  --ignore_urls="/fonts.gstatic.com/,/traefik-mesh/,/github.com\/traefik\/mesh\/edit*/,/pilot.traefik.io\/profile/,/traefik.io/,/doc.traefik.io/,/www.mkdocs.org/,/squidfunk.github.io/,/ietf.org/,/docs.github.com/,http://127.0.0.1:8000" \
  '{}' 1>/dev/null
## HTML-proofer options at https://github.com/gjtorikian/html-proofer#configuration

echo "= Documentation checked successfully."
