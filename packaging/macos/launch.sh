#!/bin/sh
# Launch the vamoose local dashboard. This is the bundle executable; it runs the vamoose
# binary bundled next to it, which serves the UI and opens the browser.
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/vamoose" app
