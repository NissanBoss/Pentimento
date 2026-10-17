package main

// What each kind of thing found is called, how loud it is, and what to do
// about it. One place, so the advice for a kind is written once and reads
// the same whichever file it turned up in.

type hitKind struct {
	tag    string
	weight Severity
	one    string // the title for one file
	many   string // the title for several, with %d
	advice string
}

// kindOrder is the order findings of the same weight are printed in.
var kindOrder = []string{
	"crop-tail", "crop-sealed", "jpeg-tail", "gif-tail",
	"original-inside", "preview-differs",
	"office-crop", "office-effect", "office-covered",
	"drawing-pages", "drawing-outside", "drawing-deleted", "drawing-crop",
	"shows-through", "mosaic",
	"motion", "depth", "phone-trailer", "tail-other",
}

var kindsOfHit = map[string]hitKind{
	"crop-tail": {
		tag: "KEPT    ", weight: Kept,
		one:  "The picture from before the crop is still in the file, behind its end",
		many: "%d files still have the picture from before the crop behind their end",
		advice: "This is how the Pixel's Markup editor and Windows' Snipping Tool saved a crop until March 2023 " +
			"(CVE-2023-21036 and CVE-2023-28303): the new picture was written over the old file without making " +
			"the file shorter. Every viewer stops where the new picture ends. Anybody with the file can decode the " +
			"rest, and the rows counted here are the ones that come back. Save the crop as a new file, from an " +
			"editor that writes one from scratch, and check that one.",
	},
	"crop-sealed": {
		tag: "CARRIES ", weight: Carries,
		one:  "The picture from before the crop is behind the end, without what it takes to decode it",
		many: "%d files have the picture from before the crop behind their end, without what it takes to decode it",
		advice: "The same mistake as CVE-2023-28303, from an encoder that writes the whole picture as one " +
			"compressed block, as Windows 10's Snip & Sketch does. The block's code table was at its start and the " +
			"new picture overwrote it, and no public tool decodes the rest without it. It is still the old picture, " +
			"and it still travels with the file: save the crop as a new file.",
	},
	"jpeg-tail": {
		tag: "KEPT    ", weight: Kept,
		one:  "The end of an earlier version of this picture is behind the end of the file",
		many: "%d files carry the end of an earlier version of themselves behind their end",
		advice: "A JPEG ends at its end marker and viewers stop there. What follows is the tail of a bigger JPEG " +
			"that was in the file before, the same mistake as the cropped screenshots of CVE-2023-28303. A JPEG is " +
			"harder to decode from the middle than a PNG, and this does not try; the bytes are still the old " +
			"picture's, and nothing about them is gone. Save a new file instead of over the old one.",
	},
	"gif-tail": {
		tag: "KEPT    ", weight: Kept,
		one:  "The end of an earlier GIF is behind the end of this one",
		many: "%d GIFs carry the end of an earlier GIF behind their end",
		advice: "The same mistake as the cropped screenshots: the new GIF was written over an older, bigger one " +
			"without making the file shorter. Save a new file instead of over the old one.",
	},
	"original-inside": {
		tag: "KEPT    ", weight: Kept,
		one:  "The photo from before the effect is inside the file",
		many: "%d files carry the photo from before the effect inside them",
		advice: "Phones keep the photo as it was taken inside the photo they made from it, so the effect can be " +
			"changed later: the background before Portrait blurred it, the face before it was relit. Anything that " +
			"shows the picture shows the effect, and the original travels with it to wherever the file goes. Save " +
			"or export a copy of the picture, which keeps only what is shown, and send that.",
	},
	"preview-differs": {
		tag: "KEPT    ", weight: Kept,
		one:  "The preview inside the file shows what the picture no longer does",
		many: "%d files carry a preview that shows what their picture no longer does",
		advice: "A JPEG carries smaller copies of itself for file browsers and cameras to show quickly, and many " +
			"editors change the picture without making those again. The part that differs is the part that was " +
			"edited, as it was before. Export a fresh copy, or strip the metadata, which takes the preview with it.",
	},
	"office-crop": {
		tag: "KEPT    ", weight: Kept,
		one:  "A picture is shown cropped, and the whole of it is in the file",
		many: "%d pictures are shown cropped, and the whole of each is in the file",
		advice: "Cropping a picture in Word, PowerPoint, Excel or LibreOffice only hides the edges. The picture " +
			"is kept whole so the crop can be undone, and whoever gets the file can undo it. In Office, Compress " +
			"Pictures with \"Delete cropped areas of pictures\" ticked takes them out for good, on the desktop apps " +
			"only.",
	},
	"office-effect": {
		tag: "KEPT    ", weight: Kept,
		one:  "A picture has an Office effect on it, and the picture before the effect is in the file",
		many: "%d pictures have an Office effect on them, and the picture before each effect is in the file",
		advice: "Word, PowerPoint and Excel keep the picture as it was beside the one with an artistic effect or " +
			"Remove Background, as an HD Photo, so the effect can be changed or reset. Blurring a face with the " +
			"Blur effect, or taking out a background, leaves the face and the background in the file. Apply the " +
			"effect in an image editor and put the result in instead.",
	},
	"office-covered": {
		tag: "KEPT    ", weight: Kept,
		one:  "A shape is drawn over a picture, and the picture is whole underneath",
		many: "%d pictures have a shape drawn over them, and are whole underneath",
		advice: "A box drawn over a picture is a second object on top of the first. Moving it, deleting it or " +
			"opening the file in anything else shows what is under it. Cover the picture in an image editor and " +
			"put the flattened picture back in.",
	},
	"drawing-pages": {
		tag: "KEPT    ", weight: Kept,
		one:  "The exported picture carries the whole diagram, pages it does not show included",
		many: "%d exported pictures carry their whole diagram, pages they do not show included",
		advice: "draw.io keeps a copy of the diagram inside the PNG or SVG it exports, so the picture can be opened " +
			"and edited again, and the copy is the whole file: every page, not only the one exported. Untick " +
			"\"Include a copy of my diagram\" when exporting, or export from a file that holds only what is meant " +
			"to be seen.",
	},
	"drawing-outside": {
		tag: "KEPT    ", weight: Kept,
		one:  "The diagram inside the picture holds shapes the picture does not show",
		many: "%d pictures hold diagrams with shapes the pictures do not show",
		advice: "The copy of the diagram kept inside an exported picture is the diagram, not the part of it that " +
			"was exported. Export without the copy of the diagram, or from a file with only what is meant to be seen.",
	},
	"drawing-deleted": {
		tag: "KEPT    ", weight: Kept,
		one:  "The drawing inside the picture still holds what was deleted from it",
		many: "%d pictures hold drawings that still have what was deleted from them",
		advice: "Excalidraw marks a deleted element as deleted and keeps it, so it can be undone and so other " +
			"people's copies agree. Some exports carry those elements in the scene embedded in the picture. Export " +
			"without \"Embed scene\", or copy what is meant to be seen into a new drawing first.",
	},
	"drawing-crop": {
		tag: "KEPT    ", weight: Kept,
		one:  "An image inside the drawing is shown cropped, and the whole of it is in the file",
		many: "%d images inside drawings are shown cropped, and the whole of each is in the file",
		advice: "Cropping an image in a drawing hides its edges and keeps the image whole inside the embedded " +
			"scene. Crop it in an image editor before putting it in, or export without the scene.",
	},
	"shows-through": {
		tag: "UNDER   ", weight: Kept,
		one:  "A box drawn over part of the picture lets what is under it through",
		many: "%d boxes drawn over pictures let what is under them through",
		advice: "A box that is not completely solid is a dimmer version of what it covers. Stretch the contrast " +
			"inside it, which any editor can do, and what was under it comes back. The factor is how far it has to " +
			"be stretched. Draw over it again with a solid colour at full opacity, and save the result as a new file.",
	},
	"mosaic": {
		tag: "MOSAIC  ", weight: Weak,
		one:  "Part of the picture is covered with a mosaic",
		many: "%d parts of pictures are covered with mosaics",
		advice: "A mosaic keeps the average colour of every block it covers. Over text in a known font, those " +
			"averages can be matched against the font rendered every possible way, and that has been done in public " +
			"(Depix, 2020; Unredacter, 2022), the smaller the blocks the easier. A solid box keeps nothing.",
	},
	"motion": {
		tag: "CARRIES ", weight: Carries,
		one:  "A video is inside the photo",
		many: "%d photos have a video inside them",
		advice: "Motion Photos on Pixel and Samsung phones record a few seconds around the shot and keep them inside " +
			"the JPEG, sound included where the phone recorded it. Anything that shows the photo shows a still; " +
			"the file still has the seconds before and after, and whatever was said in them. Turn motion off for the " +
			"shot, or send an exported copy of the still.",
	},
	"depth": {
		tag: "CARRIES ", weight: Carries,
		one:  "The photo carries a map of the scene it does not show",
		many: "%d photos carry maps of their scenes they do not show",
		advice: "Portrait and depth photos keep the distance of every part of the scene, and some keep outlines " +
			"of the people in it, so the blur can be changed later. It is the shape of the room and of the people " +
			"in it, behind whatever the blur hides. An exported copy of the picture keeps only the picture.",
	},
	"phone-trailer": {
		tag: "CARRIES ", weight: Carries,
		one:  "The phone added more after the end of the picture",
		many: "%d pictures have more after their end, added by the phone",
		advice: "Samsung phones add a trailer after the end of the picture, which most metadata tools leave alone. " +
			"Some of what it holds says where you were or what was done to the photo after it was taken.",
	},
	"tail-other": {
		tag: "CARRIES ", weight: Carries,
		one:  "Something else is behind the end of the picture",
		many: "%d pictures have something else behind their end",
		advice: "Viewers stop at the end of the picture and never show what follows it, and it travels with the " +
			"file. Something put it there on purpose, or a save went wrong. Re-export the picture to a new file to " +
			"leave it behind.",
	},
}
