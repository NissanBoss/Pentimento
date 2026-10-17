#!/bin/sh
# The bench for the two checks that read pixels. Pages of text in four
# looks, light and dark, serif and mono, each with one line covered: by a
# box at every opacity from eighty to ninety nine per cent in three
# colours, and by mosaics of every block size from four to twenty. Each is
# saved as PNG and as JPEG. The name says what was done to it:
#
#   box__colour_opacity_look.format      mosaic__block_look.format
#
# TestBench counts what is found, by each part of the name.

set -e
out="${1:?give a folder}"
mkdir -p "$out"
cd "$out"

lines='Notes from the garden committee, spring meeting
Plant the tomatoes along the south wall first
Order more compost before the end of March
Next meeting in the library, room twelve
The quick brown fox jumps over the lazy dog
Pack my box with five dozen liquor jugs'

page() { # look background ink font
    y=60
    echo "$lines" | {
        set --
        while IFS= read -r l; do
            set -- "$@" -annotate "+40+$y" "$l"
            y=$((y + 44))
        done
        convert -size 900x330 xc:"$2" -fill "$3" -font "$4" -pointsize 22 "$@" "page_$1.png"
    }
}
page light '#f6f6f0' '#1e1e28' DejaVu-Sans
page dark '#1e1e1e' '#d4d4d4' DejaVu-Sans
page serif '#ffffff' '#000000' DejaVu-Serif
page mono '#fdf6e3' '#586e75' DejaVu-Sans-Mono

for look in light dark serif mono; do
    src="page_$look.png"
    for colour in black white yellow; do
        case $colour in
            black) rgb='0,0,0' ;;
            white) rgb='255,255,255' ;;
            yellow) rgb='255,235,59' ;;
        esac
        for op in 80 85 90 93 95 97 99; do
            a=$(echo "$op" | awk '{printf "%.2f", $1/100}')
            convert "$src" -fill "rgba($rgb,$a)" -draw 'rectangle 30,82 620,112' "box__${colour}_${op}_${look}.png"
            convert "box__${colour}_${op}_${look}.png" -quality 90 "box__${colour}_${op}_${look}.jpg"
        done
    done
    for block in 4 6 8 10 12 16 20; do
        scale=$(echo "$block" | awk '{printf "%.4f", 100/$1}')
        up=$(echo "$block" | awk '{printf "%d", $1*100}')
        convert "$src" \( +clone -crop 600x40+30+78 +repage -scale "$scale%" -scale "$up%" \) \
            -geometry +30+78 -composite "mosaic__${block}_${look}.png"
        convert "mosaic__${block}_${look}.png" -quality 90 "mosaic__${block}_${look}.jpg"
    done
    mv "$src" "clean__page_$look.png"
done
ls | wc -l
