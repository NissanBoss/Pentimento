package main

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"strings"
	"testing"
)

// withText puts a tEXt chunk into a PNG, before IEND.
func withText(t *testing.T, png []byte, key, value string) []byte {
	body := append(append([]byte(key), 0), value...)
	chunk := make([]byte, 8)
	binary.BigEndian.PutUint32(chunk, uint32(len(body)))
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	end := len(png) - 12
	return append(append(append([]byte{}, png[:end]...), chunk...), png[end:]...)
}

// compressedPage is how draw.io writes a page: the model percent-encoded,
// deflated, and base64.
func compressedPage(model string) string {
	var esc strings.Builder
	for _, b := range []byte(model) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', strings.IndexByte("-_.!~*'()", b) >= 0:
			esc.WriteByte(b)
		default:
			esc.WriteString("%" + strings.ToUpper(string("0123456789abcdef"[b>>4])+string("0123456789abcdef"[b&15])))
		}
	}
	var packed bytes.Buffer
	w, _ := flate.NewWriter(&packed, 9)
	w.Write([]byte(esc.String()))
	w.Close()
	return base64.StdEncoding.EncodeToString(packed.Bytes())
}

const shownPage = `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" value="Web server" parent="1" vertex="1"/></root></mxGraphModel>`

func TestADrawioExportWithAnotherPage(t *testing.T) {
	hiddenPage := `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" value="Salaries 2027" parent="1" vertex="1"/></root></mxGraphModel>`
	mxfile := `<mxfile host="app.diagrams.net"><diagram name="Overview">` + compressedPage(shownPage) + `</diagram><diagram name="Budget">` + compressedPage(hiddenPage) + `</diagram></mxfile>`
	png := withText(t, encodePNG(t, page(120, 80, 30)), "mxfile", percentEncode(mxfile))
	c := check(t, "architecture.drawio.png", png)
	c.only(t, "drawing-pages")
	line := c.hits["drawing-pages"][0].what
	if !strings.Contains(line, "2 pages") || !strings.Contains(line, "B****t") || strings.Contains(line, "Budget") {
		t.Errorf("described as %q", line)
	}
}

func percentEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(string("0123456789abcdef"[c>>4])+string("0123456789abcdef"[c&15])))
		}
	}
	return b.String()
}

// A layer switched off before exporting is off in the picture and on in
// the copy of the diagram.
func TestADrawioSVGWithAHiddenLayer(t *testing.T) {
	model := `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="9" value="Notes" parent="0" visible="0"/>` +
		`<mxCell id="2" value="API" parent="1" vertex="1"/><mxCell id="3" value="password is hunter2" parent="9" vertex="1"/></root></mxGraphModel>`
	mxfile := `<mxfile><diagram name="Page-1">` + model + `</diagram></mxfile>`
	svg := `<svg xmlns="http://www.w3.org/2000/svg" content="` + xmlEscape(mxfile) + `" width="100" height="50"><rect width="10" height="10"/></svg>`
	c := check(t, "flow.svg", []byte(svg))
	c.only(t, "drawing-outside")
	if line := c.hits["drawing-outside"][0].what; !strings.Contains(line, "p******d") || strings.Contains(line, "hunter2") {
		t.Errorf("described as %q", line)
	}
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// Excalidraw's wrapper, as the PNG export writes it: the scene deflated
// with zlib and written as a byte string.
func wrapScene(scene string) string {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte(scene))
	w.Close()
	r := make([]rune, z.Len())
	for i, b := range z.Bytes() {
		r[i] = rune(b)
	}
	enc, _ := jsonString(string(r))
	return `{"version":"1","encoding":"bstring","compressed":true,"encoded":` + enc + `}`
}

func TestAnExcalidrawSceneWithWhatWasDeleted(t *testing.T) {
	img := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encodePNG(t, photo(60, 40, 31)))
	scene := `{"type":"excalidraw","elements":[` +
		`{"id":"a","type":"text","text":"Q3 plan","isDeleted":false},` +
		`{"id":"b","type":"text","text":"layoffs in March","isDeleted":true},` +
		`{"id":"c","type":"image","fileId":"f1","isDeleted":false,"crop":{"x":0,"y":0,"width":30,"height":40,"naturalWidth":60,"naturalHeight":40}}],` +
		`"files":{"f1":{"mimeType":"image/png","dataURL":"` + img + `"},"f2":{"mimeType":"image/png","dataURL":"` + img + `"}}}`
	png := withText(t, encodePNG(t, page(120, 80, 32)), "application/vnd.excalidraw+json", wrapScene(scene))
	c := check(t, "board.png", png)
	c.only(t, "drawing-deleted", "drawing-crop")
	all := ""
	for _, h := range c.hits["drawing-deleted"] {
		all += h.what + "\n"
	}
	if !strings.Contains(all, "l*****s") || strings.Contains(all, "layoffs") || !strings.Contains(all, "1 image") {
		t.Errorf("described as %q", all)
	}
}

func jsonString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
