package main

import (
	"strings"
	"testing"
)

const slideRels = `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image1.png"/>` +
	`<Relationship Id="rId3" Type="http://schemas.microsoft.com/office/2007/relationships/hdphoto" Target="../media/hdphoto1.wdp"/>` +
	`</Relationships>`

func slide(body string) string {
	return `<?xml version="1.0"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a14="http://schemas.microsoft.com/office/drawing/2010/main"><p:cSld><p:spTree>` + body + `</p:spTree></p:cSld></p:sld>`
}

func picture(extra string) string {
	return `<p:pic><p:blipFill><a:blip r:embed="rId2">` + extra + `</a:blip><a:srcRect l="25000" r="25000"/></p:blipFill><p:spPr><a:xfrm><a:off x="1000" y="1000"/><a:ext cx="400000" cy="300000"/></a:xfrm></p:spPr></p:pic>`
}

func shapeOver(fill string) string {
	return `<p:sp><p:spPr><a:xfrm><a:off x="100000" y="100000"/><a:ext cx="80000" cy="40000"/></a:xfrm>` + fill + `<a:ln><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:ln></p:spPr></p:sp>`
}

func deck(t *testing.T, body string) []byte {
	return officeZip(t, map[string]string{
		"[Content_Types].xml":              `<?xml version="1.0"?><Types/>`,
		"ppt/slides/slide1.xml":            slide(body),
		"ppt/slides/_rels/slide1.xml.rels": slideRels,
		"ppt/media/image1.png":             string(encodePNG(t, photo(200, 150, 20))),
		"ppt/media/hdphoto1.wdp":           "II\xbc\x01",
	})
}

func TestACroppedPictureOnASlide(t *testing.T) {
	c := check(t, "deck.pptx", deck(t, picture("")))
	c.only(t, "office-crop")
	if line := c.hits["office-crop"][0].what; !strings.Contains(line, "On slide 1") || !strings.Contains(line, "50%") {
		t.Errorf("described as %q", line)
	}
}

func TestABoxOverAPicture(t *testing.T) {
	solid := `<a:solidFill><a:srgbClr val="000000"/></a:solidFill>`
	check(t, "deck.pptx", deck(t, picture("")+shapeOver(solid))).only(t, "office-crop", "office-covered")
	// Under the picture it covers nothing, and an outline is only a line.
	check(t, "deck.pptx", deck(t, shapeOver(solid)+picture(""))).only(t, "office-crop")
	check(t, "deck.pptx", deck(t, picture("")+shapeOver(`<a:noFill/>`))).only(t, "office-crop")
}

func TestAnArtisticEffectKeepsTheOriginal(t *testing.T) {
	effect := `<a:extLst><a:ext uri="{BEBA8EAE-BF5A-486C-A8C5-ECC9F3942E4B}"><a14:imgProps><a14:imgLayer r:embed="rId3"><a14:imgEffect><a14:artisticBlur radius="40"/></a14:imgEffect></a14:imgLayer></a14:imgProps></a:ext></a:extLst>`
	c := check(t, "deck.pptx", deck(t, picture(effect)))
	c.only(t, "office-crop", "office-effect")
	if line := c.hits["office-effect"][0].what; !strings.Contains(line, "Blur effect") || !strings.Contains(line, "hdphoto1.wdp") {
		t.Errorf("described as %q", line)
	}
}

// A picture inside a document is a file like any other, and carries what
// any other file carries.
func TestAPictureInsideADocumentIsChecked(t *testing.T) {
	old := photo(640, 900, 21)
	inside := string(overwritten(encodePNG(t, cropped(old, old.Rect.Inset(200))), encodePNG(t, old)))
	doc := officeZip(t, map[string]string{
		"word/document.xml":     `<?xml version="1.0"?><w:document xmlns:w="w"><w:body/></w:document>`,
		"word/media/image1.png": inside,
	})
	c := check(t, "report.docx", doc)
	c.only(t, "crop-tail")
	if where := c.hits["crop-tail"][0].where; where != "report.docx > word/media/image1.png" {
		t.Errorf("found at %q", where)
	}
}

func TestLibreOfficeCrop(t *testing.T) {
	content := `<?xml version="1.0"?><office:document-content xmlns:office="o" xmlns:style="s" xmlns:fo="f" xmlns:draw="d" xmlns:xlink="x"><office:automatic-styles>` +
		`<style:style style:name="fr1"><style:graphic-properties fo:clip="rect(0cm, 2.5cm, 0cm, 0cm)"/></style:style>` +
		`<style:style style:name="fr2"><style:graphic-properties fo:clip="rect(0cm, 0cm, 0cm, 0cm)"/></style:style>` +
		`</office:automatic-styles><office:body><draw:frame draw:style-name="fr1"><draw:image xlink:href="Pictures/a.png"/></draw:frame>` +
		`<draw:frame draw:style-name="fr2"><draw:image xlink:href="Pictures/b.png"/></draw:frame></office:body></office:document-content>`
	odt := officeZip(t, map[string]string{
		"content.xml":    content,
		"styles.xml":     `<?xml version="1.0"?><office:document-styles xmlns:office="o"/>`,
		"Pictures/a.png": string(encodePNG(t, photo(100, 80, 22))),
		"Pictures/b.png": string(encodePNG(t, photo(100, 80, 23))),
	})
	c := check(t, "notes.odt", odt)
	c.only(t, "office-crop")
	if n := len(c.hits["office-crop"]); n != 1 {
		t.Errorf("%d crops found, and one of the two frames is cropped", n)
	}
}
