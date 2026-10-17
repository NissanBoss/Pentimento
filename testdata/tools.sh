#!/bin/sh
# Makes files the way people make them, with the tools they use: text set
# by ImageMagick, a thumbnail put in by ExifTool, a crop by jpegtran, a
# box and a mosaic drawn by ImageMagick, a crop saved over the old file
# by Python. Each file is named for what Pentimento must find in it, for
# TestMadeByTools. Run by samples.yml, on Ubuntu, with imagemagick,
# libjpeg-turbo-progs, libimage-exiftool-perl and python3-pil installed.

set -e
out="${1:?give a folder}"
mkdir -p "$out"
cd "$out"

# A page of text, like a letter or a screenshot of one.
convert -size 900x600 xc:'#f6f6f0' -fill '#1e1e28' -font DejaVu-Sans -pointsize 22 \
    -annotate +40+60 'Account holder: Marisol Etxeberria Ugarte' \
    -annotate +40+100 'IBAN ES91 2100 0418 4502 0005 1332' \
    -annotate +40+140 'Address: Calle de la Princesa 31, 4 izquierda' \
    -annotate +40+180 'Reference 8841-22-TX, due 17 November' \
    -annotate +40+260 'The quick brown fox jumps over the lazy dog 0123456789' \
    -annotate +40+300 'Pack my box with five dozen liquor jugs, twice over' \
    page.png
convert page.png -quality 92 page.jpg
convert page.png -thumbnail 160x107 thumb.jpg
exiftool -q -overwrite_original '-ThumbnailImage<=thumb.jpg' page.jpg

cp page.jpg clean__camera-like.jpg
# A box over the IBAN, drawn with the EXIF block kept as it was, which is
# what ImageMagick does by default.
convert page.jpg -fill black -draw 'rectangle 30,75 560,110' preview-differs__imagemagick-box.jpg
# A lossless crop that copies every marker, thumbnail included.
jpegtran -crop 600x200+0+40 -copy all page.jpg > preview-differs__jpegtran-crop.jpg

# A box at ninety per cent over the IBAN, and the same box solid.
convert page.png -fill 'rgba(0,0,0,0.9)' -draw 'rectangle 30,75 560,110' shows-through__imagemagick-90.png
convert page.png -fill black -draw 'rectangle 30,75 560,110' clean__imagemagick-solid.png
# A mosaic over the name, blocks of ten.
convert page.png \( +clone -crop 520x40+30+35 +repage -scale 10% -scale 1000% \) \
    -geometry +30+35 -composite mosaic__imagemagick-pixelate.png

# The crop saved over the old screenshot, with Pillow's PNG writer.
python3 - <<'PY'
from PIL import Image
old = Image.open("page.png").convert("RGB")
big = old.resize((1800, 1200))
big.save("old.png")
big.crop((100, 100, 800, 400)).save("new.png")
data = bytearray(open("old.png", "rb").read())
new = open("new.png", "rb").read()
data[:len(new)] = new
open("crop-tail__pillow-overwrite.png", "wb").write(data)
PY
rm -f page.png page.jpg thumb.jpg old.png new.png
ls -la
