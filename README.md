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
  effect can be reset.
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
  Neither reads screenshots, phone photos, slides' crops or diagrams.
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
more of the scene the copy shows.

**HEIC.** The list of what it keeps, for depth maps and mattes of the
people in a Portrait photo. Not the pixels.

**Word, PowerPoint, Excel and LibreOffice.** Every picture placed in a
document, slide or sheet: its crop, a filled shape drawn on top of it, an
artistic effect with the picture before it kept beside it, and the picture
file itself, checked like any other.

**SVG.** A draw.io copy in its `content` attribute, an Excalidraw scene in
its payload, and every picture embedded as a data URL, checked like any
other.

**The pixels.** Mosaics, found as blocks on a grid and stacked rows that
change only every block; and boxes that let what is under them through,
found as rectangles with a sharp edge all round and a faint spread of
levels inside, in strokes.

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
files from Apache POI and LibreOffice, HEIC photos from iPhones, and
phone photos from Wikimedia Commons. Forty-seven files, each with what must
be found in it and nothing else. For the two Windows 11 screenshots whose
originals were published, every byte said to come back is compared with
the original: 2,437,785 bytes in one and 378 in the other, all of them
right.

**Files made with the tools people use**, on every push: a letter set in
ImageMagick with a thumbnail put in by ExifTool, then a box drawn over a line
of it by ImageMagick, a strip cropped out by jpegtran, a box at
ninety per cent, a mosaic, and a crop saved over the old file by Pillow.

**Hundreds of ordinary files**, once a week
([`calibrate.yml`](.github/workflows/calibrate.yml)), from Wikimedia
Commons as their owners uploaded them. The last run, 17 October 2026:

- **450 Pixel photos** (Pixel 6, 7 and 8): a motion video in 25, the photo
  before Portrait Light in 1, depth maps in 2, a thumbnail showing more of
  the scene than the photo in 26, and one mosaic.
- **450 Samsung photos** (S10, S21 Ultra, S23): the phone's trailer in 244,
  nearly always with the country of the mobile network; a motion video in
  12; the unblurred shots from before Live Focus in 2; depth maps in 2; a
  thumbnail showing more than the photo in 11, and differing from it in
  one place in 2.
- **450 iPhone photos** (iPhone 12, 13 and 14): a thumbnail showing more
  than the photo in 2.
- **283 screenshots** of Android, iOS, Windows, macOS and Ubuntu: Samsung's
  trailer, naming the app that was open, in 5.
- **89 camera files** from exif-samples: a thumbnail showing more than the
  picture in 2.

Every thumbnail that shows more than its photo and was looked at by hand
was a real crop: photos trimmed before they were uploaded, in an editor
that left the thumbnail as it was. One had been cropped to take a woman
off a tram platform, and the thumbnail inside it still showed her. The
two Samsung photos that differ from their thumbnails in one place could
not be told apart, by eye, from what the phone's HDR does to a thumbnail.
That run also found four boxes over text in the screenshots and one in an
iPhone photo; none of them was one, and the rules that let them through
were changed after it (below).

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
- Of a hundred and nineteen draw.io SVGs found on GitHub, three carry
  pages the picture does not show. Of a hundred and two Excalidraw SVGs,
  none carries a deleted element (current versions leave them out of the
  export) and one carries a cropped image whole.

The one real mosaic in the photos was a karaoke machine's screen,
photographed while it showed its own pixelated preview. It was found.

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
- **In photos a camera took, boxes that let things through are not looked
  for**, because the grain of real photos made more false ones than the
  calibration ever found true ones. Screenshots, PNG or JPEG, are looked
  at, for boxes of forty pixels across and more, covering at about
  eighty five and ninety seven per cent.
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
- **The end of an earlier JPEG** is found and not decoded.
- **HEIC is read for what it keeps, not for its pixels**, which are HEVC
  and not something the standard library decodes. No public Portrait HEIC
  was found to test the mattes against, so they are recognised by the names
  Apple documents and tested on a file built here.
- **PDFs, WebP, AVIF, TIFF and Photoshop files are not read**, and are
  listed as not read. For PDFs there are Baggage and x-ray.
- **Shapes over pictures are found in PowerPoint and Excel**, where shapes
  have positions on the page. Word's floating shapes and LibreOffice's are
  not compared with the pictures under them. Slide layouts and masters
  are left out: they come with the template.
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
