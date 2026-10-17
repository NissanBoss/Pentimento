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
    -annotate +40+60 'Notes from the garden committee, spring meeting' \
    -annotate +40+100 'Plant the tomatoes along the south wall first' \
    -annotate +40+140 'Order more compost before the end of March' \
    -annotate +40+180 'Next meeting in the library, room twelve' \
    -annotate +40+260 'The quick brown fox jumps over the lazy dog 0123456789' \
    -annotate +40+300 'Pack my box with five dozen liquor jugs, twice over' \
    page.png
convert page.png -quality 92 page.jpg
convert page.png -thumbnail 160x107 thumb.jpg
exiftool -q -overwrite_original '-ThumbnailImage<=thumb.jpg' page.jpg

cp page.jpg clean__camera-like.jpg
# A box over the second line, drawn with the EXIF block kept as it was, which is
# what ImageMagick does by default.
convert page.jpg -fill black -draw 'rectangle 30,75 560,110' preview-differs__imagemagick-box.jpg
# A lossless crop that copies every marker, thumbnail included.
jpegtran -crop 600x200+0+40 -copy all page.jpg > preview-differs__jpegtran-crop.jpg

# A box at ninety per cent over the second line, and the same box solid.
convert page.png -fill 'rgba(0,0,0,0.9)' -draw 'rectangle 30,75 560,110' shows-through__imagemagick-90.png
convert page.png -fill black -draw 'rectangle 30,75 560,110' clean__imagemagick-solid.png
# A mosaic over the first line, blocks of ten.
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

# PDFs, made by PyMuPDF the way a script or a PDF tool makes them: a box
# drawn over a picture, an area marked for redaction and left at that, the
# same area redacted for real, and a filled comment box.
python3 - <<'PY'
import fitz
from PIL import Image
Image.open("crop-tail__pillow-overwrite.png").convert("RGB").save("pic.jpg", quality=90)
area = fitz.Rect(160, 140, 360, 170)

def page_with_picture():
    doc = fitz.open()
    page = doc.new_page()
    page.insert_image(fitz.Rect(100, 100, 500, 300), filename="pic.jpg")
    return doc, page

doc, page = page_with_picture()
page.draw_rect(area, color=(0, 0, 0), fill=(0, 0, 0))
doc.save("pdf-covered__pymupdf-box.pdf")

doc, page = page_with_picture()
page.add_redact_annot(area, fill=(0, 0, 0))
doc.save("pdf-redact__pymupdf-marked.pdf")
page.apply_redactions()
doc.save("clean__pymupdf-applied.pdf")

doc, page = page_with_picture()
a = page.add_rect_annot(area)
a.set_colors(stroke=(0, 0, 0), fill=(0, 0, 0))
a.update()
doc.save("pdf-annotation__pymupdf-square.pdf")
PY
rm -f pic.jpg

# A slide and two documents with cropped pictures, exported to PDF by
# LibreOffice: the export puts every picture in whole and clips it, so
# the crop survives into the PDF.
for f in 54542_cropped_bitmap.pptx; do
    curl -sSfL -o "$f" "https://raw.githubusercontent.com/apache/poi/598e21ca4ee6e9f34d79a0c7411f8e6c802512c6/test-data/slideshow/$f"
done
curl -sSfL -o croppedAndRotated.odt "https://raw.githubusercontent.com/LibreOffice/core/c5089c5db6897db4a7e49e3e526b01db34ed6c02/sw/qa/extras/ooxmlexport/data/croppedAndRotated.odt"
curl -sSfL -o ImageCrop.docx "https://raw.githubusercontent.com/LibreOffice/core/c5089c5db6897db4a7e49e3e526b01db34ed6c02/sw/qa/extras/ooxmlexport/data/ImageCrop.docx"
soffice --headless --convert-to pdf 54542_cropped_bitmap.pptx croppedAndRotated.odt ImageCrop.docx >/dev/null 2>&1 || true
for f in 54542_cropped_bitmap croppedAndRotated ImageCrop; do
    [ -s "$f.pdf" ] && mv "$f.pdf" "pdf-crop__libreoffice-$f.pdf"
    rm -f "$f.pptx" "$f.odt" "$f.docx"
done
ls -la

# A Writer document with a filled box drawn in front of a picture, anchored in
# the picture's paragraph, and the same document saved as Word by
# LibreOffice.
python3 - <<'PY'
import zipfile
from PIL import Image
Image.open("crop-tail__pillow-overwrite.png").convert("RGB").resize((400, 300)).save("a.png")
ns = ('xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" '
      'xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" '
      'xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" '
      'xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" '
      'xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" '
      'xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" '
      'xmlns:xlink="http://www.w3.org/1999/xlink"')
content = ('<?xml version="1.0" encoding="UTF-8"?><office:document-content ' + ns + ' office:version="1.3">'
    '<office:automatic-styles><style:style style:name="gr1" style:family="graphic">'
    '<style:graphic-properties draw:fill="solid" draw:fill-color="#000000" style:wrap="run-through" style:run-through="foreground"/></style:style></office:automatic-styles>'
    '<office:body><office:text><text:p>'
    '<draw:frame draw:name="Picture" text:anchor-type="paragraph" svg:x="0cm" svg:y="0cm" svg:width="12cm" svg:height="9cm" draw:z-index="0">'
    '<draw:image xlink:href="Pictures/a.png" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/></draw:frame>'
    '<draw:rect draw:style-name="gr1" text:anchor-type="paragraph" svg:x="2cm" svg:y="3cm" svg:width="6cm" svg:height="1.5cm" draw:z-index="1"/>'
    '</text:p></office:text></office:body></office:document-content>')
manifest = ('<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3">'
    '<manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>'
    '<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>'
    '<manifest:file-entry manifest:full-path="Pictures/a.png" manifest:media-type="image/png"/></manifest:manifest>')
with zipfile.ZipFile("office-covered__writer-box.odt", "w") as z:
    z.writestr(zipfile.ZipInfo("mimetype"), "application/vnd.oasis.opendocument.text")
    z.writestr("META-INF/manifest.xml", manifest)
    z.writestr("content.xml", content)
    z.write("a.png", "Pictures/a.png")
PY
rm -f a.png
cp office-covered__writer-box.odt boxed.odt
soffice --headless --convert-to docx boxed.odt >/dev/null 2>&1 || true
[ -s boxed.docx ] && mv boxed.docx office-covered__libreoffice-word-box.docx
rm -f boxed.odt
ls -la

# The same box behind the text, which Word draws under the picture: it
# covers nothing, and must not be reported.
python3 - <<'PY'
import zipfile
src = zipfile.ZipFile("office-covered__writer-box.odt")
with zipfile.ZipFile("behind.odt", "w") as z:
    for item in src.infolist():
        data = src.read(item.filename)
        if item.filename == "content.xml":
            data = data.replace(b'style:run-through="foreground"', b'style:run-through="background"')
        z.writestr(item, data)
PY
soffice --headless --convert-to docx behind.odt >/dev/null 2>&1 || true
[ -s behind.docx ] && mv behind.docx clean__libreoffice-word-box-behind.docx
rm -f behind.odt
