# Changelog

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
