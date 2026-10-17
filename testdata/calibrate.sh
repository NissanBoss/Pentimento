#!/bin/sh
# Downloads the calibration set into a folder, one subfolder per group:
# photos from phones and screenshots from Wikimedia Commons, as their
# owners uploaded them, and the camera files of exif-samples. Run by
# calibrate.yml on GitHub's machines; it is a few gigabytes, and is not
# something to run on a laptop.
#
#   sh testdata/calibrate.sh /tmp/cal 150
#   PENTIMENTO_CALIBRATION=/tmp/cal go test -run Calibration -v -timeout 60m

set -e
out="${1:?give a folder to download into}"
per="${2:-150}"
agent="pentimento-calibration/1 (https://github.com/NissanBoss/Pentimento)"
api="https://commons.wikimedia.org/w/api.php"

# list <category> <how many> prints the original URLs of its files.
list() {
    cont=""
    got=0
    while [ "$got" -lt "$2" ]; do
        page=$(curl -sS -A "$agent" -G "$api" \
            --data-urlencode "gcmtitle=Category:$1" \
            -d action=query -d generator=categorymembers -d gcmtype=file \
            -d gcmlimit=50 -d prop=imageinfo -d iiprop=url -d format=json \
            ${cont:+--data-urlencode "gcmcontinue=$cont"})
        urls=$(printf '%s' "$page" | grep -o '"url":"[^"?]*' | sed 's/"url":"//' | grep -i '\.\(jpe\?g\|png\)$' || true)
        [ -n "$urls" ] || break
        for u in $urls; do
            [ "$got" -lt "$2" ] || break
            # Flickr imports went through Flickr, which writes its own file;
            # only what the owner uploaded says what the phone wrote.
            case "$u" in *%28[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]%29*) continue ;; esac
            echo "$u"
            got=$((got + 1))
        done
        cont=$(printf '%s' "$page" | grep -o '"gcmcontinue":"[^"]*' | sed 's/"gcmcontinue":"//')
        [ -n "$cont" ] || break
        sleep 1
    done
}

fetch() {
    group="$1"
    mkdir -p "$out/$group"
    n=0
    while read -r u; do
        n=$((n + 1))
        f="$out/$group/$n.${u##*.}"
        [ -s "$f" ] && continue
        for try in 1 2 3 4; do
            curl -sSfL -A "$agent" -o "$f" "$u" || true
            if [ -s "$f" ] && ! head -c 64 "$f" | grep -qi '<!doctype html\|<html'; then
                break
            fi
            rm -f "$f"
            sleep $((try * 5))
        done
        sleep 0.5
    done
}

for c in Taken_with_Google_Pixel_7 Taken_with_Google_Pixel_6 Taken_with_Google_Pixel_8; do
    list "$c" "$per"
done | fetch pixel
for c in Taken_with_Samsung_Galaxy_S23 Taken_with_Samsung_Galaxy_S10 Taken_with_Samsung_Galaxy_S21_Ultra_5G; do
    list "$c" "$per"
done | fetch samsung
for c in Taken_with_iPhone_13 Taken_with_iPhone_14 Taken_with_iPhone_12; do
    list "$c" "$per"
done | fetch iphone
for c in Screenshots_of_Android_12 Screenshots_of_Android_13 Screenshots_of_Android_14 Windows_10_screenshots Windows_7_screenshots MacOS_screenshots IOS_screenshots Wikipedia_iOS_app_screenshots Commons_iOS_app_screenshots Screenshots_of_Ubuntu_GNOME; do
    list "$c" "$((per / 3))"
done | fetch screenshots

mkdir -p "$out/cameras"
gh_raw="https://raw.githubusercontent.com/ianare/exif-samples/5332a9cf0220e9c6c93f88a187daa90808051e10"
curl -sS "https://api.github.com/repos/ianare/exif-samples/git/trees/5332a9cf0220e9c6c93f88a187daa90808051e10?recursive=1" |
    grep -o '"path": *"jpg/[^"]*\.jpg"' | sed 's/"path": *"//; s/"$//' | while read -r p; do
        curl -sSfL -o "$out/cameras/$(echo "$p" | tr '/' '_')" "$gh_raw/$p" || true
    done

for g in pixel samsung iphone screenshots cameras; do
    echo "$g: $(ls "$out/$g" | wc -l) files, $(du -sh "$out/$g" | cut -f1)"
done
