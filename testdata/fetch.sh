#!/bin/sh
# Downloads the files listed in samples.tsv into a folder, for samples.yml
# and for running the same check by hand:
#
#   sh testdata/fetch.sh /tmp/samples
#   PENTIMENTO_SAMPLES=/tmp/samples go test -run Samples -v
#
# Wikimedia asks for a user agent that says who is asking, and throttles
# anything that asks too fast; a throttled download is a page of HTML,
# which is caught by looking at what came back rather than trusted.

set -e
out="${1:?give a folder to download into}"
mkdir -p "$out"
here="$(dirname "$0")"
agent="pentimento-samples/1 (https://github.com/NissanBoss/Pentimento)"
grep -v '^#' "$here/samples.tsv" | while IFS="$(printf '\t')" read -r name url expect original; do
    [ -n "$name" ] || continue
    [ -s "$out/$name" ] && continue
    for try in 1 2 3 4 5; do
        curl -sSfL -A "$agent" -o "$out/$name" "$url" || true
        if [ -s "$out/$name" ] && ! head -c 64 "$out/$name" | grep -qi '<!doctype html\|<html'; then
            break
        fi
        rm -f "$out/$name"
        sleep $((try * 3))
    done
    [ -s "$out/$name" ] || { echo "could not download $name"; exit 1; }
    sleep 1
done
echo "$(ls "$out" | wc -l) files in $out"
