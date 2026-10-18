# Changelog

## v1.1

What v1 named as its weak points, worked on.

- **PDF:** every picture on every page, for a filled rectangle drawn over
  it (not when the pixels under it are blank, which is what an applied
  redaction leaves), a clip showing only part of it, redactions marked and
  never applied, filled annotations over the page, and every JPEG in the
  file checked like any other. LibreOffice's PDF export of a cropped
  picture, which puts the whole picture in, is found that way.
- **The end of an earlier JPEG is decoded**, with the tables of the
  picture in front: how many rows of the old picture are there and how
  wide it was, checked against the Snipping Tool's published originals.
- **Word and Writer:** a filled shape over a picture anchored in the same
  paragraph, or both on the page, in front of it.
- **Boxes nearly the colour of the page**, black over dark mode, white or
  yellow over a light page, found by their edges, with disabled buttons
  and ruled backgrounds told apart.
- **Mosaics in JPEG**, which v1 found none of on the bench, in pictures
  of screens, sixteen blocks across and up; photos are held to v1's
  rules. Screens are told from photos by their perfectly flat blocks, not
  by EXIF, which chat apps strip, and boxes are no longer looked for in
  photos without EXIF either.
- **A picture's own grid**, a game scaled up three times, is no longer
  taken for mosaics.
- **A JPEG of grey text is read as a JPEG**, where v1 read it as a PNG.
- **A thumbnail that shows more** is backed up by the EXIF: when the photo
  is smaller than the camera made it, that is said; when it is the size
  the camera made it and was saved when it was taken, the report says the
  phone most likely did it, as a digital zoom does.
- **The bench:** 112 covers on four kinds of page, each saved as PNG and
  as JPEG, counted on every push, with floors the counts may not drop
  under. Boxes found: 57 of 84 in PNG, 33 in JPEG (v1: 26 and 20).
  Mosaics: 28 of 28 and 17 (v1: 25 and none).
- **The calibration** is three times the size, one job per group, also
  looks at every photo as if its EXIF were gone, and logs what the EXIF
  says about how each photo with a thumbnail finding was saved.

## v1

The first release.

Says whether what you cropped, covered or blurred out of a file is still in
it, and how much of it.

- **Crops saved over the old file** (CVE-2023-21036, CVE-2023-28303): the
  old screenshot behind the end of a PNG, decoded from the middle of its
  compressed stream by a decoder written for this, with its width found
  from the rows themselves and every pixel that comes back counted exactly.
  The end of an earlier JPEG or GIF behind the end of one, and Windows 10's
  leftovers that cannot be decoded, told apart.
- **Copies of the picture inside it:** the EXIF thumbnail and the
  Multi-Picture images, each found inside the picture and compared with it,
  for what a crop or a zoom took off and for a part covered since; the
  photo before Portrait's blur, Portrait Light and Lens Blur on Pixels, and
  before Live Focus on Samsungs; motion videos, with their length and
  whether they have sound; depth maps and mattes.
- **Samsung's trailer:** the country of the mobile network, the edits kept
  so they can be undone, and in screenshots, the app that was open.
- **Office and LibreOffice:** crops that keep the whole picture, filled
  shapes over pictures, artistic effects and Remove Background with the
  picture before them kept, and every picture inside checked like a file.
- **Diagrams:** the pages and hidden layers in draw.io's copy of the
  diagram, and the deleted elements, cropped images and unused images in
  Excalidraw's, in PNG and SVG.
- **In the pixels:** mosaics, and boxes that let what is under them
  through.
- **Checked against real files** on every push, from where they were
  published, with the bytes recovered from cropped screenshots compared
  with their originals; against files made by ImageMagick, ExifTool,
  jpegtran and Pillow; and once a week against hundreds of phone photos
  and screenshots from Wikimedia Commons.
- **It only reads:** nothing is written, nothing is sent, no other program
  is run, with tests that read the source to keep it that way. Words found
  in files and the home folder are masked unless `--reveal` is given.
