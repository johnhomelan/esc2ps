package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseTextAndFonts(t *testing.T) {
	// Construct an ESC/P2 data stream:
	// "Hello " + ESC E (bold on) + "World" + ESC F (bold off) + CR + LF + ESC k 1 (Sans Serif) + "Goodbye"
	var buf bytes.Buffer
	buf.WriteString("Hello ")
	buf.Write([]byte{0x1B, 'E'}) // Bold on
	buf.WriteString("World")
	buf.Write([]byte{0x1B, 'F'}) // Bold off
	buf.Write([]byte{0x0D, 0x0A}) // CR + LF
	buf.Write([]byte{0x1B, 'k', 1}) // Select SansSerif typeface
	buf.WriteString("Goodbye")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	// Check items
	// Since we coalesce matching font/Y items, we should have:
	// Item 0: "Hello " (Courier, normal)
	// Item 1: "World" (Courier, Bold)
	// Item 2: "Goodbye" (SansSerif, normal)
	// Note: the CR + LF moves Y, so "Goodbye" is on next line and cannot be coalesced.
	if len(page.Items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(page.Items))
	}

	// Verify Item 0
	t1, ok := page.Items[0].(*TextItem)
	if !ok {
		t.Fatalf("Item 0 is not a TextItem")
	}
	if t1.Text != "Hello " {
		t.Errorf("Expected 'Hello ', got '%s'", t1.Text)
	}
	if t1.Font.Bold {
		t.Errorf("Expected normal font, got bold")
	}
	if t1.Font.Typeface != "Courier" {
		t.Errorf("Expected Courier font, got %s", t1.Font.Typeface)
	}

	// Verify Item 1
	t2, ok := page.Items[1].(*TextItem)
	if !ok {
		t.Fatalf("Item 1 is not a TextItem")
	}
	if t2.Text != "World" {
		t.Errorf("Expected 'World', got '%s'", t2.Text)
	}
	if !t2.Font.Bold {
		t.Errorf("Expected bold font")
	}
	if t2.Font.Typeface != "Courier" {
		t.Errorf("Expected Courier font, got %s", t2.Font.Typeface)
	}

	// Verify Item 2 (after CR+LF)
	t3, ok := page.Items[2].(*TextItem)
	if !ok {
		t.Fatalf("Item 2 is not a TextItem")
	}
	if t3.Text != "Goodbye" {
		t.Errorf("Expected 'Goodbye', got '%s'", t3.Text)
	}
	if t3.Font.Bold {
		t.Errorf("Expected normal font, got bold")
	}
	if t3.Font.Typeface != "SansSerif" {
		t.Errorf("Expected SansSerif font, got %s", t3.Font.Typeface)
	}
}

func Test8DotGraphics(t *testing.T) {
	// Construct a stream with ESC * 0 4 0 (8-dot single density, 4 columns)
	// Followed by 4 bytes of column data: 0x80 (top dot only), 0x01 (bottom dot only), 0x55, 0xAA
	var buf bytes.Buffer
	buf.Write([]byte{0x1B, '*', 0, 4, 0})
	buf.Write([]byte{0x80, 0x01, 0x55, 0xAA})

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	if len(page.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(page.Items))
	}

	img, ok := page.Items[0].(*ImageItem)
	if !ok {
		t.Fatalf("Expected item to be an ImageItem")
	}

	if img.ImgW != 4 {
		t.Errorf("Expected width 4, got %d", img.ImgW)
	}
	if img.ImgH != 8 {
		t.Errorf("Expected height 8, got %d", img.ImgH)
	}

	// Row 0 has top pixel (from 0x80 on col 0, 0x55 on col 2: bit 7 is 0, wait, 0x55 is 01010101, so bit 7 is 0, bit 6 is 1, bit 5 is 0, bit 4 is 1...
	// Column 0: 0x80 -> 10000000. So Row 0 has col 0.
	// Column 1: 0x01 -> 00000001. So Row 7 has col 1.
	// Column 2: 0x55 -> 01010101. So Rows 1, 3, 5, 7 have col 2.
	// Column 3: 0xAA -> 10101010. So Rows 0, 2, 4, 6 have col 3.
	// Row bytes for width 4 is (4+7)/8 = 1 byte per row.
	// Let's verify bitmap bytes:
	// Row 0: col 0, col 3. In 1-byte row, MSB is col 0, bit 4 is col 3 -> 10010000 (0x90).
	if img.Data[0] != 0x90 {
		t.Errorf("Expected Row 0 to be 0x90, got 0x%02X", img.Data[0])
	}
	// Row 7: col 1, col 2. MSB is col 0 (0), col 1 (1), col 2 (1) -> 01100000 (0x60).
	if img.Data[7] != 0x60 {
		t.Errorf("Expected Row 7 to be 0x60, got 0x%02X", img.Data[7])
	}
}

func Test24DotGraphics(t *testing.T) {
	// ESC * 32 2 0 (24-dot single density, 2 columns)
	// Column 0: 0xFF, 0x00, 0xAA
	// Column 1: 0x00, 0xFF, 0x55
	var buf bytes.Buffer
	buf.Write([]byte{0x1B, '*', 32, 2, 0})
	buf.Write([]byte{0xFF, 0x00, 0xAA, 0x00, 0xFF, 0x55})

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	page := pages[0]
	img := page.Items[0].(*ImageItem)

	if img.ImgW != 2 || img.ImgH != 24 {
		t.Fatalf("Expected 2x24 image, got %dx%d", img.ImgW, img.ImgH)
	}
}

func TestRasterGraphics(t *testing.T) {
	// ESC . c v h m nL nH (Raster Graphics)
	// ESC . 0 10 10 4 8 0 [data] (4 rows, 8 columns = 1 byte per row, 4 bytes total)
	var buf bytes.Buffer
	buf.Write([]byte{0x1B, '.', 0, 10, 10, 4, 8, 0})
	buf.Write([]byte{0xAA, 0x55, 0xF0, 0x0F})

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	page := pages[0]
	img := page.Items[0].(*ImageItem)

	if img.ImgW != 8 || img.ImgH != 4 {
		t.Fatalf("Expected 8x4 image, got %dx%d", img.ImgW, img.ImgH)
	}

	if img.Data[0] != 0xAA || img.Data[1] != 0x55 || img.Data[2] != 0xF0 || img.Data[3] != 0x0F {
		t.Errorf("Expected [0xAA, 0x55, 0xF0, 0x0F], got [%02X, %02X, %02X, %02X]", img.Data[0], img.Data[1], img.Data[2], img.Data[3])
	}
}

func TestPostScriptGeneration(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Hello ")
	buf.Write([]byte{0x1B, 'E'})
	buf.WriteString("World")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	var psBuf bytes.Buffer
	if err := GeneratePostScript(pages, &psBuf); err != nil {
		t.Fatalf("Failed to generate PS: %v", err)
	}

	psOutput := psBuf.String()
	// Basic checks on PS structure
	if !strings.HasPrefix(psOutput, "%!PS-Adobe-3.0") {
		t.Errorf("Expected standard DSC header")
	}
	if !strings.Contains(psOutput, "Courier-Bold") {
		t.Errorf("Expected Bold Courier font usage")
	}
	if !strings.Contains(psOutput, "showpage") {
		t.Errorf("Expected showpage command")
	}
}

func TestColorPrinting(t *testing.T) {
	// ESC r n selects color: 0=black, 1=magenta, 2=cyan, etc.
	var buf bytes.Buffer
	buf.WriteString("BlackText ")
	buf.Write([]byte{0x1B, 'r', 1}) // Magenta
	buf.WriteString("MagentaText ")
	buf.Write([]byte{0x1B, 'r', 2}) // Cyan
	buf.WriteString("CyanText")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	// Since color is a property of FontInfo, each color change results in a separate TextItem.
	// 3 text items: "BlackText ", "MagentaText ", "CyanText"
	if len(page.Items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(page.Items))
	}

	// Verify items
	ti0 := page.Items[0].(*TextItem)
	if ti0.Font.Color != 0 {
		t.Errorf("Expected first item to be color 0 (black), got %d", ti0.Font.Color)
	}

	ti1 := page.Items[1].(*TextItem)
	if ti1.Font.Color != 1 {
		t.Errorf("Expected second item to be color 1 (magenta), got %d", ti1.Font.Color)
	}

	ti2 := page.Items[2].(*TextItem)
	if ti2.Font.Color != 2 {
		t.Errorf("Expected third item to be color 2 (cyan), got %d", ti2.Font.Color)
	}

	// Generate PS and verify color codes are present
	var psBuf bytes.Buffer
	if err := GeneratePostScript(pages, &psBuf); err != nil {
		t.Fatalf("Failed to generate PS: %v", err)
	}

	psOutput := psBuf.String()
	// Magenta is 1.0 0.0 1.0 setrgbcolor
	if !strings.Contains(psOutput, "1.000000 0.000000 1.000000 setrgbcolor") {
		t.Errorf("Expected magenta color command in PostScript")
	}
	// Cyan is 0.0 1.0 1.0 setrgbcolor
	if !strings.Contains(psOutput, "0.000000 1.000000 1.000000 setrgbcolor") {
		t.Errorf("Expected cyan color command in PostScript")
	}
}

func TestInitializePrinterEscAt(t *testing.T) {
	// ESC @ is called at start and end of document. Ensure pages are not wiped out.
	var buf bytes.Buffer
	buf.Write([]byte{0x1B, '@'}) // Initialize printer at start
	buf.WriteString("PageContent")
	buf.Write([]byte{0x1B, '@'}) // Initialize printer at end

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	if len(page.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(page.Items))
	}

	textItem, ok := page.Items[0].(*TextItem)
	if !ok {
		t.Fatalf("Expected first item to be a TextItem")
	}

	if textItem.Text != "PageContent" {
		t.Errorf("Expected 'PageContent', got '%s'", textItem.Text)
	}
}

func TestUserDefinedCharacters(t *testing.T) {
	var buf bytes.Buffer
	// Define custom char 65 ('A')
	buf.Write([]byte{0x1B, '&', 0, 65, 65})
	buf.Write([]byte{0x00, 0x02}) // attr=0, width=2
	buf.Write([]byte{0xFF, 0x00, 0xAA, 0x00, 0xFF, 0x55}) // 6 bytes pattern

	// Select user defined character set
	buf.Write([]byte{0x1B, '%', 1})
	buf.WriteString("A")

	// Select standard character set
	buf.Write([]byte{0x1B, '%', 0})
	buf.WriteString("A")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	// Should have 2 items: custom "A" and standard "A"
	if len(page.Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(page.Items))
	}

	ti0 := page.Items[0].(*TextItem)
	if !ti0.Font.UserFont {
		t.Errorf("Expected first item to use UserFont")
	}

	ti1 := page.Items[1].(*TextItem)
	if ti1.Font.UserFont {
		t.Errorf("Expected second item to NOT use UserFont")
	}

	// Verify the UserChar exists in page metadata
	if len(page.UserChars) != 1 {
		t.Fatalf("Expected 1 defined user character on the page, got %d", len(page.UserChars))
	}

	uc := page.UserChars[65]
	if uc == nil {
		t.Fatalf("Custom character 65 not found")
	}
	if uc.Width != 2 {
		t.Errorf("Expected width 2, got %d", uc.Width)
	}

	// Verify PostScript output contains the custom font definition
	var psBuf bytes.Buffer
	if err := GeneratePostScript(pages, &psBuf); err != nil {
		t.Fatalf("Failed to generate PS: %v", err)
	}

	psOutput := psBuf.String()
	if !strings.Contains(psOutput, "/EpsonUserFont exch definefont pop") {
		t.Errorf("Expected EpsonUserFont definition in PostScript")
	}
	if !strings.Contains(psOutput, "/EpsonUserFont findfont") {
		t.Errorf("Expected EpsonUserFont font selection in PostScript")
	}
	if !strings.Contains(psOutput, "imagemask") {
		t.Errorf("Expected imagemask command inside custom glyph definition")
	}
}

func TestInternationalCharSetUK(t *testing.T) {
	// ESC R 3 selects the UK table, which maps '#' (0x23) to '£' (0xA3).
	var buf bytes.Buffer
	buf.Write([]byte{0x1B, 'R', 3})
	buf.WriteString("Price: #5.00")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(pages) != 1 || len(pages[0].Items) != 1 {
		t.Fatalf("Expected 1 page with 1 item, got pages=%d", len(pages))
	}

	ti, ok := pages[0].Items[0].(*TextItem)
	if !ok {
		t.Fatalf("Item 0 is not a TextItem")
	}

	want := "Price: \xa35.00"
	if ti.Text != want {
		t.Errorf("Expected %q (£ as raw byte 0xA3), got %q (bytes: % X)", want, ti.Text, []byte(ti.Text))
	}

	// The PostScript output must re-encode fonts to ISOLatin1Encoding and
	// use the -Latin1 font name so byte 0xA3 renders as sterling.
	var psBuf bytes.Buffer
	if err := GeneratePostScript(pages, &psBuf); err != nil {
		t.Fatalf("Failed to generate PS: %v", err)
	}
	psOutput := psBuf.String()
	if !strings.Contains(psOutput, "/Courier-Latin1 findfont") {
		t.Errorf("Expected Courier-Latin1 font selection in PostScript")
	}
	if !strings.Contains(psOutput, "/Encoding ISOLatin1Encoding def") {
		t.Errorf("Expected ISOLatin1Encoding re-encoding block in PostScript")
	}
	if !strings.Contains(psOutput, `\243`) {
		t.Errorf("Expected octal-escaped byte \\243 (0xA3) for £ in PostScript output")
	}
}

func TestInternationalCharSetDefaultUSAUnchanged(t *testing.T) {
	// CharSet defaults to 0 (USA), which must leave every "national" code
	// byte untouched - this is the regression guard for plain ESC/P content.
	var buf bytes.Buffer
	buf.WriteString("#$@[\\]^`{|}~")

	pages, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(pages) != 1 || len(pages[0].Items) != 1 {
		t.Fatalf("Expected 1 page with 1 item, got pages=%d", len(pages))
	}

	ti := pages[0].Items[0].(*TextItem)
	want := "#$@[\\]^`{|}~"
	if ti.Text != want {
		t.Errorf("Expected default USA set to leave text unchanged: got %q, want %q", ti.Text, want)
	}
}
