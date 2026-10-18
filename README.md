# Pentimento

Whether what you cropped, covered or blurred out of a file is really gone:
the old screenshot still behind the end of the cropped one, the photo from
before the blur still inside the portrait, the whole picture still inside
the slide that shows a corner of it, the text still showing through a box
that is not quite solid.

A pentimento is the earlier painting that shows through when the paint on
top of it thins with age. Files do the same thing, and sooner.

```
  Pentimento: whether what you took out of these files is gone
  ============================================================

  Looked at 2 JPEGs, 2 documents and 3 PNGs.

    [KEPT    ] The picture from before the crop is still in the file, behind its end
              ~/to-send/screenshot.png
                The bottom 669 rows of an earlier picture 1,366 pixels wide,
                behind this one of 353x241.
                431 of the rows come back exactly, and 67% of the pixels
                across all of them.

    [KEPT    ] The photo from before the effect is inside the file
              ~/to-send/PXL_PORTRAIT.jpg
                The photo before Portrait blurred it, 2016x1512, 783 KB, in
                the Google container, as android/original_image.

    [KEPT    ] 2 pictures have an Office effect on them, and the picture before each
              effect is in the file
              ~/to-send/report.docx
                In the document, the picture image1.png (32x32) has Office's
                Texturizer effect on it, and the picture before it is in the
                file as hdphoto1.wdp.

    [MOSAIC  ] Part of the picture is covered with a mosaic
              ~/to-send/token.png
                240x20 at 5,0, in blocks of 5 pixels, 48 across and 4 down.

    [CARRIES ] A video is inside the photo
              ~/to-send/PXL_MP.jpg
                A 1.6-second video, without sound, 2048x1536, 2.7 MB, in the
                Google container, as MotionPhoto.

    [CARRIES ] The phone added more after the end of the picture
              ~/to-send/Screenshot_app.png
                The app the screenshot was taken in:
                c*m.t*******f.c***t/.M**********y.
```

That is a real run on real files, cut short; each finding is followed by
what to do about it, left out here. The screenshot is one of the Windows 11
Snipping Tool files published with the detector for CVE-2023-28303; the two
photos are from Pixels, uploaded to Wikimedia Commons by their owners; the
document is a LibreOffice test file; the mosaic is one of Depix's own test
pictures; the screenshot of an app is from a Samsung phone, also on
Commons. Every one of them is checked again on every push, from where it
was published.

## The problem

You crop a screenshot before posting it in an issue. You draw a box over
an account number, pixelate a token, blur the background of a photo of
your flat, crop a colleague out of a slide. On the screen it is done: the
part you took out is not there.

In the file it often is.

- **A crop saved over the old file.** In March 2023 Simon Aarons and David
  Buchanan found that the Pixel's Markup editor wrote a cropped screenshot
  over the old file without making the file shorter (CVE-2023-21036), and
  then that Windows' Snipping Tool did the same (CVE-2023-28303). Every
  viewer stops at the end of the new picture; the old one is behind it,
  and most of it can be decoded. Years of screenshots were saved that
  way before the fixes, and the files are still wherever they were sent.
- **The thumbnail.** A JPEG carries a small copy of itself, and many
  editors change the picture without making the copy again. Crop out a
  person, box out a name: the thumbnail still has them. In the calibration
  below, a photo on Wikimedia Commons had been cropped to take a woman off
  a tram platform, and the thumbnail inside it still showed her.
- **The phone's own copies.** A Pixel's Portrait photo keeps the photo as
  it was taken, before the blur, inside the file, so the blur can be
  changed later. Portrait Light keeps the face before it was relit.
  Samsung's Live Focus keeps the unblurred shots. Motion Photos keep a few
  seconds of video around the shot. None of it is shown by anything that
  shows the photo.
- **Office.** Cropping a picture in Word or PowerPoint hides its edges and
  keeps it whole. A box drawn over a picture is a second object on top of
  it. An artistic effect, Blur among them, and Remove Background, keep the
  picture as it was beside the one with the effect, as an HD Photo, so the
  effect can be reset. Export the document to PDF with LibreOffice and the
  cropped picture goes into the PDF whole, shown through a clip.
- **PDF.** A black rectangle drawn over a picture is drawn after it, and
  the picture is still there underneath. A redaction marked in a PDF
  editor and never applied is a note saying where the redaction would go.
  A filled comment box is an annotation, and lifts off.
- **Diagrams.** draw.io and Excalidraw put a copy of the drawing inside
  the PNG or SVG they export so it can be edited again, and the copy is the
  drawing, not the picture: every page, every hidden layer, the whole of
  every cropped image.
- **The cover itself.** A box at ninety per cent opacity is a dimmer copy
  of what it covers; stretch the contrast and it comes back. A mosaic
  keeps the average of every block, and averages of text in a known font
  have been matched in public (Depix, Unredacter).

Each of these is known to somebody. None of them is something you check
before pressing send, because nothing checks them together.

## What already exists

- [acropalypse.app](https://acropalypse.app), the
  [Acropalypse Multi-Tool](https://github.com/frankthetank-music/Acropalypse-Multi-Tool)
  and [infobyte's detector](https://github.com/infobyte/CVE-2023-21036)
  answer the crop bug alone. They detect it as bytes after the end of the
  PNG, and the tools that recover need you to type in the width the old
  screenshot had. Pentimento works out the width itself, says how many
  rows come back and how many of their pixels exactly, never guesses a
  pixel, and tells apart the Windows 10 files that cannot be decoded at
  all.
- [ExifTool](https://exiftool.org) reads nearly everything here, the
  thumbnail, the Google container, Samsung's trailer, and prints it. It
  does not compare a thumbnail with its picture or say that a field holds
  the photo before the blur; it was never meant to.
- [x-ray](https://github.com/freelawproject/x-ray) finds text under black
  boxes in PDFs, and [unmasker](https://github.com/osint-shifu/unmasker)
  does that and more in documents, with a look at a JPEG's thumbnail.
  Neither reads screenshots, phone photos, slides' crops or diagrams, and
  in a PDF both look at text. Pentimento looks at the pictures in it: a
  box over a picture, a clip that hides most of one, a redaction marked
  and not applied, and every JPEG inside, thumbnail and all. Text under
  boxes it leaves to them. x-ray's own test files are among the samples,
  and the ones it calls clean are clean here too.
- [Baggage](https://github.com/NissanBoss/Baggage), from this same series,
  says what metadata a file carries, and notices a thumbnail of a
  different shape and bytes after the end. Pentimento is the other half:
  whether those bytes, and that thumbnail, are the part you took out.
- [Depix](https://github.com/spipm/Depix) and
  [Unredacter](https://github.com/BishopFox/unredacter) recover pixelated
  text. They are why a mosaic is reported here, and they are attacks, not
  checks.

## What it looks for

```bash
pentimento screenshot.png
pentimento ~/Desktop/to-send/
```

The kind of a file is decided by its first bytes, not its name. A folder is
walked for the kinds below and nothing else is opened.

**PNG and GIF.** Where the picture really ends, and what is after it. When
that is the tail of an older picture, the old compressed data is taken out
of what is left of its chunks, a point is found where a compressed block
begins, and it is decoded from there by a decoder written for this, which
starts in the middle of a stream and marks every byte copied from before
the start as unknown. The width of the old picture is found from the filter
byte every row starts with. The report says how many rows of the old
picture are in the file and how many of their pixels come back exactly;
nothing is guessed. Text chunks are read for a draw.io or Excalidraw copy.

**JPEG.** The EXIF thumbnail and the Multi-Picture index; base64 in the
XMP, where Google kept the photo before Lens Blur and before Portrait
Light; the files a Google container lists after the end, with what each
one is for; Samsung's trailer, read the way ExifTool's Samsung module reads
it; and whatever else is after the end. Every copy of the picture is found
inside the picture, at whatever size and place fits, and compared with it,
for a part where the picture is flat and the copy is not, and for how much
more of the scene the copy shows; and when the picture is smaller than its
EXIF says the camera made it, that is said too, as editors mostly copy the
EXIF as it was. The end of an earlier JPEG behind this one, which is what
the Snipping Tool leaves when it saves a crop as JPEG, is decoded with the
tables of the picture in front, which the two share: the report says how
many rows of the old picture are there and how wide it was. Those rows are
exact but for their overall brightness and tint, which were in the part
the new picture wrote over.

**PDF.** Every picture drawn on every page: a filled rectangle drawn over
it afterwards, unless the pixels under the rectangle are blank, which is
what a redaction that was applied leaves; a clip that shows only part of
it; areas marked for redaction and never redacted; filled annotations over
the page; and every JPEG in the file, checked like any other.

**HEIC.** The list of what it keeps, for depth maps and mattes of the
people in a Portrait photo. Not the pixels.

**Word, PowerPoint, Excel and LibreOffice.** Every picture placed in a
document, slide or sheet: its crop, a filled shape drawn on top of it, an
artistic effect with the picture before it kept beside it, and the picture
file itself, checked like any other. In Word and Writer, where shapes sit
in the flow of the text, a shape and a picture are compared when both are
anchored in the same paragraph, or both to the page, which is where a box
drawn over a picture ends up; a shape behind the text covers nothing and
is not reported.

**SVG.** A draw.io copy in its `content` attribute, an Excalidraw scene in
its payload, and every picture embedded as a data URL, checked like any
other.

**The pixels.** Mosaics, found as blocks on a grid and stacked rows that
change only every block; and boxes that let what is under them through,
found as rectangles with a sharp edge all round and a faint spread of
levels inside, in strokes. A box nearly the colour of the page, black over
dark mode or white over white, has no edge sharp enough to end a region
at, and is found instead by its edges: straight lines along which the
colour steps by the same few levels, with square corners, which a
disabled button's rounded ones are not, and a step that holds a few pixels
either side, which a ruled line's does not. In a JPEG, a mosaic's edges
can be a pixel off the grid, and that is allowed only in a picture of a
screen, told by its perfectly flat blocks rather than by its EXIF, since
a phone photo sent through a chat app has none left; and a mosaic in a
JPEG has to be sixteen blocks across. Mosaics that all sit on one grid of
the whole picture are its own pixels, as in a game scaled up, and are
left out.

The exit code is 1 when something taken out is still in a file, 0 when
nothing turned up, 2 when the files could not be opened, and 3 when
nothing given was a kind it reads.

## The calibration

Every rule here was tried on real files before it was kept, and most of
them were changed by it. The comments in the code say which file changed
which rule.

**Real files with a known answer**, checked on every push
([`samples.yml`](.github/workflows/samples.yml)): the Snipping Tool's own
cropped files with their originals, the demonstration files of a public
recovery tool, Depix's test pictures, Excalidraw's fixtures, Office test
files from Apache POI and LibreOffice, HEIC photos from iPhones, phone
photos from Wikimedia Commons, and x-ray's test PDFs. Sixty-one files, each
with what must be found in it and nothing else. For the two Windows 11
screenshots whose originals were published, every byte said to come back
is compared with the original: 2,437,785 bytes in one and 378 in the
other, all of them right. For the two Snipping Tool JPEGs whose originals
were published, the rows decoded from the old picture's end are compared
with the same rows of the original, and correlate at 0.9985 and 0.9999.

**Files made with the tools people use**, on every push: a letter set in
ImageMagick with a thumbnail put in by ExifTool, then a box drawn over a line
of it by ImageMagick, a strip cropped out by jpegtran, a box at
ninety per cent, a mosaic, and a crop saved over the old file by Pillow; PDFs
made by PyMuPDF with a box over a picture, a redaction marked and the same
one applied, and a filled comment box; a slide and two documents with
cropped pictures exported to PDF by LibreOffice; and a Writer document with
a box over its picture, saved as Word by LibreOffice, in front of the
picture and behind it.

**A bench of covers**, on every push
([`bench.sh`](testdata/bench.sh)): pages of text in four looks, light,
dark, serif and mono, each with one line covered by a box at every opacity
from eighty to ninety nine per cent in black, white and yellow, and by a
mosaic of every block size from four to twenty, saved as PNG and as JPEG
at ninety. What is found:

| | PNG | JPEG |
|---|---|---|
| Boxes that let the line through | 57 of 84 | 33 of 84 |
| Mosaics | 28 of 28 | 17 of 28 |

None of the clean pages has anything found on it. A box at ninety nine per
cent is two levels from solid and is never found; nor is a white box on a
pure white page, which has no edge at all. In a JPEG, a box under about
ten levels from the page is lost in the noise around the text, and mosaics
of blocks smaller than JPEG's own eight are not looked for, nor ones
narrower than sixteen blocks. The test fails if any of the four totals
drops below a floor a little under these, so a change that loses some
has to say so. The first release found 26 and 20 of the boxes and 25 and
none of the mosaics.

**Thousands of ordinary files**, once a week
([`calibrate.yml`](.github/workflows/calibrate.yml)), from Wikimedia
Commons as their owners uploaded them, one job per group. Every photo is
also looked at a second time as if its EXIF were gone, which is how a
chat app sends it. The last run, 18 October 2026:

- **1,060 Pixel photos** (Pixel 6, 7 and 8): a motion video in 96, the
  photo before Portrait or Portrait Light in 16, depth maps in 8, a
  thumbnail showing more of the scene than the photo in 35, and one
  mosaic.
- **1,350 Samsung photos** (S10, S21 Ultra, S23): the phone's trailer in
  664, nearly always with the country of the mobile network; a motion
  video in 22; the unblurred shots from before Live Focus in 2; depth maps
  in 2; a thumbnail showing more than the photo, or differing from it in
  one place, in 16.
- **1,350 iPhone photos** (iPhone 12, 13 and 14): a thumbnail showing more
  than the photo in 17, and two mosaics.
- **641 screenshots** of Android, iOS, Windows, macOS and Ubuntu: Samsung's
  trailer, naming the app that was open, in 5; four boxes over text and
  one mosaic.
- **89 camera files** from exif-samples: a thumbnail showing more than the
  picture in 2.

With their EXIF gone, the photos gave one more mosaic and one more box
between them. Of the JPEGs, 92 of the 126 screenshots were taken for
pictures of a screen, and 16 of the 3,838 photos.

The thumbnails were checked against what the EXIF says. In 18 of the 71,
the photo is smaller than the EXIF says the camera made it: crops, and 10
of them iPhone photos cropped and saved by iOS itself, which left the
thumbnail of the whole photo in. In 45, the photo is the size the camera
made it, saved by the phone's own software when it was taken, and in
series: Pixel photos whose thumbnails all show the same eleven or twenty
two per cent more, Samsung photos the same six or seven. That is the
phone, most likely its digital zoom, and the report says so; the thumbnail
still has what the photo leaves out. The rest were saved later by an
editor, or have no size to go by. Earlier runs looked at by hand found a
photo cropped to take a woman off a tram platform, with her still in the
thumbnail.

The mosaic in the Pixel photos is most likely the one the first release
found, a karaoke machine's screen showing its own pixelated preview. The
two in iPhone photos and the boxes in the screenshots were not looked at
by hand; the first release's run reported four boxes in fewer
screenshots, and none of those was a cover.

What the calibration taught, all of it in the code:

- A Pixel's thumbnail is made before the lens is corrected, so it shows
  four or five per cent more than the photo at every edge, and things near
  the edges are a few pixels apart in the two. Before that was understood,
  one Pixel photo in seven came out as cropped or changed.
- A page of text shrunk to a thumbnail matches itself badly, a strip
  cropped from a letter scoring 0.56 in the right place; the first version
  did not find jpegtran's crop at all.
- In a photo, rectangles with faint marks in them are signs, windows,
  labels and the thick strokes of big letters flecked with dust; thirteen
  of them in the first nine hundred photos, and one more later, looked like
  boxes over text, and not one real box turned up in thirteen hundred and
  fifty. So in photos a camera took, boxes that let things through are no
  longer looked for. In a JPEG, a hard-compressed sky is blocks of eight on
  JPEG's own grid, which is a mosaic in every way but the size of the steps
  between blocks.
- In screenshots, a toolbar button greyed out because it cannot be used is
  a dimmer copy of its icon, which is exactly what a box that lets things
  through is. Icons are now too small to count, and so are spreads of
  levels too faint or too strong for any cover.
- Pixel 4a to 6 photos end with tens of kilobytes of the camera app's own
  tuning data, and Pixels list their motion video a little shorter than
  it is.
- Samsung's screenshots carry the same trailer as its photos, and in a
  screenshot it names the app that was open.
- On six hundred and forty seven screenshots of the author's own, bars of
  grey with a hairline every eight pixels, drawn by a program on every
  line of one window, came out as fifteen boxes over text. Text under a
  cover is marks of every shape, and a pattern of lines from side to side
  is no longer taken for it. After that, nothing turned up in any of them.
- Looking for boxes nearly the colour of the page, the same screenshots
  gave forty one: disabled buttons, which are dim rectangles with dim
  text and rounded corners, and the rows of a background ruled in squares,
  whose edges are lines. A drawn box has square corners and an edge that
  holds a few pixels either side, and after that nothing turned up again.
- Letting a JPEG's mosaic be a pixel off its grid, as the bench needs,
  found hundreds of mosaics in phone photos: JPEG's own blocks of eight in
  smooth places, lines of text, grass. So photos keep the first release's
  rules, a JPEG is held to the looser ones only when it is a picture of a
  screen, told by its perfectly flat blocks, and a mosaic in a JPEG must be
  sixteen blocks across. Telling screens by blocks flat to three levels
  took a fifth of the iPhone photos for screens, once their EXIF was off.
- A game drawn in big pixels and scaled up three times is a mosaic from
  edge to edge; one screenshot came out as twenty. Mosaics on one grid of
  the whole picture are now taken for its own.
- Of a hundred and nineteen draw.io SVGs found on GitHub, three carry
  pages the picture does not show. Of a hundred and two Excalidraw SVGs,
  none carries a deleted element (current versions leave them out of the
  export) and one carries a cropped image whole.


## Running it

Download the package for your system from the Releases page, unpack it,
and run the binary. There is nothing to install.

```bash
pentimento ~/Desktop/to-send/
```

```bash
pentimento --reveal screenshot.png
```

The home folder is shown as `~`, and the words found in files (a page's
name, a label, the app a screenshot was taken in) are masked to their
first and last letters, so the report can be pasted somewhere. `--reveal`
shows both.

Windows marks anything that came from the internet, so the first run may
bring up a blue "Windows protected your PC" box. Right click the zip,
Properties, tick Unblock at the bottom, and then extract it. Building it
yourself avoids the question.

## What it cannot do

- **It does not hand you what it finds.** It says the old screenshot is
  behind the end, how much of it and where; it does not write it out. A
  report gets pasted into issues and chats, and a tool that wrote the
  recovered picture next to it would be the leak it was looking for.
- **A blur is not looked for, and neither is a part painted over with
  something plausible.** Telling a blur somebody applied from a part of a
  photo out of focus, and a filled-in background from a real one, is not
  something this can do well enough to say. A blur that kept the original
  in the file, as Portrait does, is found by the original.
- **In photos, boxes that let things through are not looked for**, with
  their EXIF or without it, because the grain of real photos made more
  false ones than the calibration ever found true ones. Screenshots are
  looked at, for boxes of forty pixels across and more, covering at
  between about eighty and ninety seven per cent: every PNG, and a JPEG
  when it is told for a picture of a screen, which a quarter of the
  calibration's JPEG screenshots were not. The bench above says how many
  boxes are found.
- **A mosaic in a JPEG is found when it is sixteen blocks across or
  more, with blocks of eight pixels and up.** A word pixelated small in a
  JPEG is missed; looser rules found hundreds of mosaics in ordinary
  photos.
- **A cover at the very edge of a photo is missed by the thumbnail
  comparison**, in the outer six per cent, where lens correction moves a
  photo against its own thumbnail.
- **What comes back of a cropped screenshot depends on the encoder.** Rows
  the compressor wrote as copies of rows that are gone cannot be had
  exactly; the public tools fill them with a guess, and this counts only
  what is exact. One of the Windows 11 files gives back under one per cent
  exactly. Windows 10's Snip & Sketch wrote whole screenshots as one
  compressed block, and its leftovers cannot be decoded by this or by any
  public tool: they are reported as there, not as readable.
- **The end of an earlier JPEG is decoded only when it shares the tables
  of the picture in front**, as the Snipping Tool's do, and only when it
  is baseline JPEG; otherwise it is found and not decoded. Its brightness
  and tint are not known, as they were in the part written over.
- **HEIC is read for what it keeps, not for its pixels**, which are HEVC
  and not something the standard library decodes. No public Portrait HEIC
  was found to test the mattes against, so they are recognised by the names
  Apple documents and tested on a file built here.
- **In a PDF, text under a box is not looked for**, only pictures; that is
  what Baggage and x-ray do. WebP, AVIF, TIFF and Photoshop files are not
  read, and are listed as not read.
- **Shapes over pictures in Word and Writer are compared only when they
  are placed from the same thing**, the same paragraph or the page; a
  shape placed from the margin over a picture placed in a line is left
  alone rather than guessed at. Slide layouts and masters are left out:
  they come with the template.
- **draw.io's copy is checked for pages and hidden layers**, not for what
  lies outside the area exported.

## It only reads

- **Nothing is written.** Not a recovered picture, not a report file, not
  a cache. There are tests that read the source and fail the build if a
  call that writes, deletes or renames anything appears in it.
- **Nothing is sent.** There is no network code in it, and a test fails
  the build if any appears. It does not run other programs either.
- **A gap is not a pass.** A file it could not read, a kind it does not
  read, a picture it could not decode: each is said, first, and a report
  with a gap in it never calls itself clear.
- **Nothing is guessed.** A byte of an old screenshot is counted as back
  only when it is exactly what it was, which the calibration checks
  against the originals, byte for byte.
- **Doing it right is not a finding.** A solid box, a crop saved as a new
  file, a thumbnail that matches its picture, the gain map an HDR photo
  needs: none of them is reported.

## Building

Needs [Go](https://go.dev) and nothing else. No dependencies at all.

```bash
go build -o pentimento .
```

```bash
go test ./...
```

```bash
sh testdata/fetch.sh /tmp/samples
PENTIMENTO_SAMPLES=/tmp/samples go test -run TestSamples -v
```

The tests build the files they check, so what makes each case is in the
test and not in a binary nobody can read. The real files are downloaded,
never kept in the repository: they are other people's.

## Licence

MIT. Do what you like with it.
