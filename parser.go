package main

import (
	"fmt"
	"io"
)

type trackingReader struct {
	r   io.Reader
	off int64
}

func (tr *trackingReader) Read(p []byte) (int, error) {
	n, err := tr.r.Read(p)
	tr.off += int64(n)
	return n, err
}

// Item represents a printable element on a page (text or graphic).
type Item interface {
	IsItem()
}

// FontInfo contains styling and formatting information for text.
type FontInfo struct {
	Typeface     string  // "Courier", "Roman", "SansSerif"
	Size         float64 // Font size in PostScript points (1/72 inch)
	Bold         bool
	Italic       bool
	Underline    bool
	DoubleStrike bool
	SuperSub     int  // -1 for subscript, 1 for superscript, 0 for normal
	Pitch        int  // CPI: 10, 12, 15, or -1 for proportional
	DoubleWidth  bool
	DoubleHeight bool
	Condensed    bool
	Color        int  // 0: Black, 1: Magenta, 2: Cyan, 3: Violet, 4: Yellow, 5: Orange, 6: Green
	UserFont     bool
}

// TextItem represents a string of text printed at a specific position.
type TextItem struct {
	X    int      // X coordinate in 1/21600 inch (from left of page)
	Y    int      // Y coordinate in 1/21600 inch (from top of page)
	Text string
	Font FontInfo
}

func (t *TextItem) IsItem() {}

// ImageItem represents a raster graphic printed at a specific position.
type ImageItem struct {
	X      int    // X coordinate in 1/21600 inch
	Y      int    // Y coordinate in 1/21600 inch
	Width  int    // Graphic width in 1/21600 inch
	Height int    // Graphic height in 1/21600 inch
	ImgW   int    // Width in pixels
	ImgH   int    // Height in pixels
	Data   []byte // Bitmap data (1 bit per pixel, row-by-row, MSB first)
}

func (i *ImageItem) IsItem() {}

// Page represents a single printed page.
type Page struct {
	Width     int // Page width in 1/21600 inch
	Height    int // Page height in 1/21600 inch
	Items     []Item
	UserChars map[byte]*UserChar // Keep track of custom characters defined in the document
}

// UserChar represents a custom-defined character bitmap.
type UserChar struct {
	Width  int    // width in columns (W)
	Bitmap []byte // row-by-row bitmap (height is always 24)
}

// ParserState holds the current printer settings and coordinates.
type ParserState struct {
	Pages       []*Page
	CurrentPage *Page

	// Current position (in 1/21600 inch)
	X int
	Y int

	// Page size (in 1/21600 inch)
	PageWidth  int // Letter default = 8.5" * 21600 = 183600
	PageHeight int // Letter default = 11.0" * 21600 = 237600

	// Margins (in 1/21600 inch)
	LeftMargin   int
	RightMargin  int
	TopMargin    int
	BottomMargin int

	// Motion and Layout
	LineSpacing int // in 1/21600 inch
	HorizUnit   int // units per inch (for ESC $ and relative, default 60)
	VertUnit    int // units per inch (for ESC ( V and relative, default 360)

	// User-Defined Font properties
	UserChars           map[byte]*UserChar
	UseUserDefinedChars bool

	// Font properties
	Typeface            string  // "Courier", "Roman", "SansSerif"
	Size                float64 // font size in points
	Bold                bool
	Italic              bool
	Underline           bool
	DoubleStrike        bool
	SuperSub            int // -1 subscript, 1 superscript, 0 normal
	Pitch               int // 10, 12, 15, or -1 for proportional
	Proportional        bool
	DoubleWidth         bool
	DoubleHeight        bool
	Condensed           bool
	IntercharacterSpace int // in 1/21600 inch
	Color               int // 0: Black, 1: Magenta, 2: Cyan, 3: Violet, 4: Yellow, 5: Orange, 6: Green

	// Temporary states
	DoubleWidthOneLine bool
	Tabs               []int
}

// NewParserState initializes the default ESC/P2 printer state.
func NewParserState() *ParserState {
	s := &ParserState{
		PageWidth:           183600, // 8.5 inches
		PageHeight:          237600, // 11 inches
		LineSpacing:         3600,   // 1/6 inch (6 lines per inch)
		HorizUnit:           60,     // 1/60 inch
		VertUnit:            360,    // 1/360 inch
		Typeface:            "Courier",
		Size:                10.0,
		Pitch:               10,
		Proportional:        false,
		UserChars:           make(map[byte]*UserChar),
		UseUserDefinedChars: false,
	}
	s.LeftMargin = 0
	s.RightMargin = s.PageWidth
	return s
}

// getPage retrieves or lazily initializes the current page.
func (s *ParserState) getPage() *Page {
	if s.CurrentPage == nil {
		s.CurrentPage = &Page{
			Width:     s.PageWidth,
			Height:    s.PageHeight,
			UserChars: make(map[byte]*UserChar),
		}
		s.Pages = append(s.Pages, s.CurrentPage)
		s.X = s.LeftMargin
		s.Y = 0
	}
	return s.CurrentPage
}

// activeFont returns the current font settings.
func (s *ParserState) activeFont() FontInfo {
	return FontInfo{
		Typeface:     s.Typeface,
		Size:         s.Size,
		Bold:         s.Bold,
		Italic:       s.Italic,
		Underline:    s.Underline,
		DoubleStrike: s.DoubleStrike,
		SuperSub:     s.SuperSub,
		Pitch:        s.Pitch,
		DoubleWidth:  s.DoubleWidth || s.DoubleWidthOneLine,
		DoubleHeight: s.DoubleHeight,
		Condensed:    s.Condensed,
		Color:        s.Color,
	}
}

// charWidth returns the horizontal advance of a character in 1/21600 inch.
func (s *ParserState) charWidth() int {
	var baseWidth int
	if s.Proportional {
		// Use proportional width estimation
		// Approx 0.5 inches per 72 points of font size
		baseWidth = int((s.Size / 72.0) * 21600.0 * 0.5)
	} else {
		// Monospaced pitch
		pitch := s.Pitch
		if s.Condensed {
			switch pitch {
			case 10:
				baseWidth = 21600 / 17 // ~17.14 CPI
			case 12:
				baseWidth = 21600 / 20 // 20 CPI
			case 15:
				baseWidth = 21600 / 25 // 25 CPI
			default:
				baseWidth = 21600 / 17
			}
		} else {
			switch pitch {
			case 10:
				baseWidth = 21600 / 10 // 2160 units
			case 12:
				baseWidth = 21600 / 12 // 1800 units
			case 15:
				baseWidth = 21600 / 15 // 1440 units
			default:
				baseWidth = 21600 / 10
			}
		}
	}

	if s.DoubleWidth || s.DoubleWidthOneLine {
		baseWidth *= 2
	}

	return baseWidth + s.IntercharacterSpace
}

// addText appends a string to the current page, coalescing text items if possible.
func (s *ParserState) addText(text string) {
	if text == "" {
		return
	}
	for i := 0; i < len(text); i++ {
		ch := text[i]
		chStr := string(ch)
		page := s.getPage()

		font := s.activeFont()
		font.UserFont = s.UseUserDefinedChars && s.UserChars[ch] != nil

		w := s.charWidth()

		// Try to append to the last item on the page if the font, row and alignment match
		coalesced := false
		if len(page.Items) > 0 {
			if lastText, ok := page.Items[len(page.Items)-1].(*TextItem); ok {
				if lastText.Font == font && lastText.Y == s.Y {
					expectedX := lastText.X + len(lastText.Text)*w
					if lastText.X <= s.X && s.X <= expectedX+w {
						// Fill gap with spaces if needed
						gap := s.X - expectedX
						if gap > 0 && w > 0 {
							spaces := gap / w
							for idx := 0; idx < spaces; idx++ {
								lastText.Text += " "
							}
						}
						lastText.Text += chStr
						s.X += w
						coalesced = true
					}
				}
			}
		}

		if !coalesced {
			// Create new text item
			item := &TextItem{
				X:    s.X,
				Y:    s.Y,
				Text: chStr,
				Font: font,
			}
			page.Items = append(page.Items, item)
			s.X += w
		}
	}
}

// Parse parses ESC/P2 data from the reader and returns a list of pages.
func Parse(r io.Reader) ([]*Page, error) {
	tr := &trackingReader{r: r}
	s := NewParserState()
	buf := make([]byte, 1)

	for {
		n, err := tr.Read(buf)
		if n == 0 {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read error at offset %d: %w", tr.off, err)
		}

		b := buf[0]

		switch b {
		case 0x00: // NUL
			// Ignore NUL
		case 0x07: // BEL
			// Ignore
		case 0x08: // BS (Backspace)
			s.X -= s.charWidth()
			if s.X < s.LeftMargin {
				s.X = s.LeftMargin
			}
		case 0x09: // HT (Horizontal Tab)
			// Move to next tab stop or standard tab stops (every 8 characters)
			tabW := s.charWidth() * 8
			if tabW <= 0 {
				tabW = 21600 * 8 / 10 // default 10 cpi
			}
			nextX := ((s.X / tabW) + 1) * tabW
			s.X = nextX
		case 0x0A: // LF (Line Feed)
			s.Y += s.LineSpacing
			s.DoubleWidthOneLine = false // Cancel one-line double width
			// Check page overflow
			if s.Y > s.PageHeight-s.BottomMargin {
				s.CurrentPage = nil // Force new page on next draw
			}
		case 0x0B: // VT (Vertical Tab)
			s.Y += s.LineSpacing // treat as line feed for simplicity
		case 0x0C: // FF (Form Feed)
			s.CurrentPage = nil // Force new page
		case 0x0D: // CR (Carriage Return)
			s.X = s.LeftMargin
		case 0x0E: // SO (Double-width for 1 line)
			s.DoubleWidthOneLine = true
		case 0x0F: // SI (Condensed printing)
			s.Condensed = true
		case 0x12: // DC2 (Cancel condensed)
			s.Condensed = false
		case 0x14: // DC4 (Cancel one-line double-width)
			s.DoubleWidthOneLine = false
		case 0x18: // CAN (Cancel line)
			// Clears buffer, but in this parsed model, just reset horizontal position
			s.X = s.LeftMargin
		case 0x1B: // ESC
			if err := s.handleESC(tr); err != nil {
				return nil, fmt.Errorf("error handling ESC at offset %d: %w", tr.off, err)
			}
		default:
			// Printable character
			s.addText(string(b))
		}
	}

	// Ensure all pages have the final complete set of UserChars
	for _, page := range s.Pages {
		page.UserChars = s.UserChars
	}

	return s.Pages, nil
}

// handleESC parses escape sequences.
func (s *ParserState) handleESC(r io.Reader) error {
	cmd := make([]byte, 1)
	if _, err := io.ReadFull(r, cmd); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil // Gracefully handle truncated trailing ESC
		}
		return err
	}

	switch cmd[0] {
	case 0x0E: // ESC SO (Double-width 1 line)
		s.DoubleWidthOneLine = true
	case 0x0F: // ESC SI (Condensed)
		s.Condensed = true
	case 0x20: // ESC SP n (Intercharacter space)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		// n is in 1/120 inch or similar. Let's assume 1/120 inch
		s.IntercharacterSpace = int(val) * (21600 / 120)
	case '!': // ESC ! n (Master select)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		// Bit 0: 10 cpi (0) / 12 cpi (1)
		if val&0x01 != 0 {
			s.Pitch = 12
		} else {
			s.Pitch = 10
		}
		// Bit 1: Proportional (1)
		s.Proportional = (val & 0x02) != 0
		// Bit 2: Condensed (1)
		s.Condensed = (val & 0x04) != 0
		// Bit 3: Bold (1)
		s.Bold = (val & 0x08) != 0
		// Bit 4: Double-width (1)
		s.DoubleWidth = (val & 0x10) != 0
		// Bit 5: Double-strike (1)
		s.DoubleStrike = (val & 0x20) != 0
		// Bit 6: Italic (1)
		s.Italic = (val & 0x40) != 0
		// Bit 7: Underline (1)
		s.Underline = (val & 0x80) != 0

	case '$': // ESC $ nL nH (Absolute horizontal position)
		nL, err := readByte(r)
		if err != nil {
			return err
		}
		nH, err := readByte(r)
		if err != nil {
			return err
		}
		pos := int(nL) + int(nH)*256
		// Convert pos from s.HorizUnit dpi to 21600 units
		s.X = s.LeftMargin + pos*(21600/s.HorizUnit)

	case '%': // ESC % n (Select user-defined character set)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.UseUserDefinedChars = val == 1 || val == 49

	case '&': // ESC & (Define user-defined characters)
		// ESC & 0x00 first last [data]
		if err := s.defineUserDefinedChars(r); err != nil {
			return err
		}

	case '*': // ESC * m nL nH (Select bit image)
		if err := s.handleBitImage(r); err != nil {
			return err
		}

	case '-': // ESC - n (Underline)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.Underline = val == 1 || val == 49

	case '/': // ESC / c (Select V-tab channel)
		_, _ = readByte(r) // skip

	case '0': // ESC 0 (Line spacing 1/8 inch)
		s.LineSpacing = 21600 / 8 // 2700 units

	case '2': // ESC 2 (Line spacing 1/6 inch)
		s.LineSpacing = 21600 / 6 // 3600 units

	case '3': // ESC 3 n (Line spacing n/180 inch)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.LineSpacing = int(val) * (21600 / 180) // 120 units per n

	case '4': // ESC 4 (Select Italic)
		s.Italic = true

	case '5': // ESC 5 (Cancel Italic)
		s.Italic = false

	case '@': // ESC @ (Initialize printer)
		pages := s.Pages
		currentPage := s.CurrentPage
		*s = *NewParserState()
		s.Pages = pages
		s.CurrentPage = currentPage

	case 'A': // ESC A n (Line spacing n/60 inch)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.LineSpacing = int(val) * (21600 / 60)

	case 'B', 'D': // ESC B / D (Vertical/Horizontal tab stops)
		// Read until 0x00
		for {
			val, err := readByte(r)
			if err != nil {
				return err
			}
			if val == 0 {
				break
			}
		}

	case 'C': // ESC C [0] n (Page length)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		if val == 0 {
			// ESC C 0 n (page length in inches)
			n, err := readByte(r)
			if err != nil {
				return err
			}
			s.PageHeight = int(n) * 21600
		} else {
			// page length in lines
			s.PageHeight = int(val) * s.LineSpacing
		}
		s.RightMargin = s.PageWidth

	case 'E': // ESC E (Select Bold/Emphasized)
		s.Bold = true

	case 'F': // ESC F (Cancel Bold)
		s.Bold = false

	case 'G': // ESC G (Select Double-strike)
		s.DoubleStrike = true

	case 'H': // ESC H (Cancel Double-strike)
		s.DoubleStrike = false

	case 'I': // ESC I n (Select character table)
		_, _ = readByte(r) // skip

	case 'J': // ESC J n (Feed paper n/180 inch)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.Y += int(val) * (21600 / 180)

	case 'K', 'L', 'Y', 'Z': // ESC 8-dot graphics equivalents
		// Translate to equivalent ESC * modes
		var m byte
		switch cmd[0] {
		case 'K':
			m = 0 // single density
		case 'L':
			m = 1 // double density
		case 'Y':
			m = 2 // double speed double density
		case 'Z':
			m = 3 // quadruple density
		}
		nL, err := readByte(r)
		if err != nil {
			return err
		}
		nH, err := readByte(r)
		if err != nil {
			return err
		}
		cols := int(nL) + int(nH)*256
		data := make([]byte, cols)
		if _, err := io.ReadFull(r, data); err != nil {
			return err
		}
		s.addBitImage8(int(m), cols, data)

	case 'M': // ESC M (Select 12 CPI)
		s.Pitch = 12
		s.Proportional = false

	case 'N': // ESC N n (Skip-over perforation)
		_, _ = readByte(r)

	case 'O': // ESC O (Cancel skip-over perforation)
		// skip

	case 'P': // ESC P (Select 10 CPI)
		s.Pitch = 10
		s.Proportional = false

	case 'Q': // ESC Q n (Set right margin)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.RightMargin = int(val) * s.charWidth()

	case 'R': // ESC R n (International character set)
		_, _ = readByte(r)

	case 'S': // ESC S n (Select sub/superscript)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		if val == 0 || val == 48 {
			s.SuperSub = 1 // superscript
		} else {
			s.SuperSub = -1 // subscript
		}

	case 'T': // ESC T (Cancel sub/superscript)
		s.SuperSub = 0

	case 'U': // ESC U n (Unidirectional)
		_, _ = readByte(r)

	case 'W': // ESC W n (Double-width)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.DoubleWidth = val == 1 || val == 49

	case 'X': // ESC X m nL nH (Select scalable font & size)
		m, err := readByte(r)
		if err != nil {
			return err
		}
		nL, err := readByte(r)
		if err != nil {
			return err
		}
		nH, err := readByte(r)
		if err != nil {
			return err
		}
		sizeHalfPt := int(nL) + int(nH)*256
		s.Size = float64(sizeHalfPt) / 2.0
		if s.Size <= 0 {
			s.Size = 10.0
		}
		// Map typeface
		switch m {
		case 0:
			s.Typeface = "Roman"
		case 1:
			s.Typeface = "SansSerif"
		case 2:
			s.Typeface = "Courier"
		default:
			s.Typeface = "Courier"
		}

	case '\\': // ESC \ nL nH (Relative horizontal position)
		nL, err := readByte(r)
		if err != nil {
			return err
		}
		nH, err := readByte(r)
		if err != nil {
			return err
		}
		offset := int(int16(uint16(nL) | (uint16(nH) << 8)))
		s.X += offset * (21600 / s.HorizUnit)

	case 'a': // ESC a n (Alignment)
		_, _ = readByte(r)

	case 'd': // ESC d n (Relative vertical paper feed)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.Y += int(val) * (21600 / 180)

	case 'g': // ESC g (Select 15 CPI)
		s.Pitch = 15
		s.Proportional = false

	case 'k': // ESC k n (Select typeface)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		switch val {
		case 0:
			s.Typeface = "Roman"
		case 1:
			s.Typeface = "SansSerif"
		default:
			s.Typeface = "Courier"
		}

	case 'l': // ESC l n (Set left margin)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.LeftMargin = int(val) * s.charWidth()
		if s.X < s.LeftMargin {
			s.X = s.LeftMargin
		}

	case 'p': // ESC p n (Proportional mode on/off)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.Proportional = val == 1 || val == 49

	case 'r': // ESC r n (Select color)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.Color = int(val)

	case 't': // ESC t n (Select character table)
		_, _ = readByte(r)

	case 'w': // ESC w n (Double height)
		val, err := readByte(r)
		if err != nil {
			return err
		}
		s.DoubleHeight = val == 1 || val == 49

	case 'x': // ESC x n (Draft/LQ)
		_, _ = readByte(r)

	case '(': // ESC ( ... multi-byte / ESC/P2 extension
		if err := s.handleGroupCommand(r); err != nil {
			return err
		}

	case '.': // ESC . c v h m nL nH (Raster graphics)
		if err := s.handleRasterGraphics(r); err != nil {
			return err
		}
	}

	return nil
}

// defineUserDefinedChars parses and registers custom character definitions for ESC &
func (s *ParserState) defineUserDefinedChars(r io.Reader) error {
	// Format: ESC & 0x00 first last [data...]
	_, err := readByte(r) // skip 0x00 / format byte
	if err != nil {
		return err
	}
	first, err := readByte(r)
	if err != nil {
		return err
	}
	last, err := readByte(r)
	if err != nil {
		return err
	}

	count := int(last - first + 1)
	if count < 0 {
		return nil
	}

	for i := 0; i < count; i++ {
		charCode := first + byte(i)
		attr, err := readByte(r)
		if err != nil {
			return err
		}
		w, err := readByte(r)
		if err != nil {
			return err
		}

		cols := int(w)
		data := make([]byte, cols*3)
		if _, err := io.ReadFull(r, data); err != nil {
			return err
		}

		// Convert 24 vertical dots per column to row-by-row bitmap (24 rows high)
		rowBytes := (cols + 7) / 8
		bitmap := make([]byte, 24*rowBytes)

		for col := 0; col < cols; col++ {
			b0 := data[3*col]
			b1 := data[3*col+1]
			b2 := data[3*col+2]

			for row := 0; row < 24; row++ {
				var bit byte
				if row < 8 {
					bit = (b0 >> (7 - uint(row))) & 1
				} else if row < 16 {
					bit = (b1 >> (7 - uint(row-8))) & 1
				} else {
					bit = (b2 >> (7 - uint(row-16))) & 1
				}

				if bit != 0 {
					byteIdx := row*rowBytes + (col / 8)
					bitIdx := 7 - uint(col%8)
					bitmap[byteIdx] |= (1 << bitIdx)
				}
			}
		}

		s.UserChars[charCode] = &UserChar{
			Width:  cols,
			Bitmap: bitmap,
		}
		_ = attr
	}
	return nil
}

// handleGroupCommand parses commands starting with ESC (
func (s *ParserState) handleGroupCommand(r io.Reader) error {
	cmdType := make([]byte, 1)
	if _, err := io.ReadFull(r, cmdType); err != nil {
		return err
	}

	// Read length nL nH
	nL, err := readByte(r)
	if err != nil {
		return err
	}
	nH, err := readByte(r)
	if err != nil {
		return err
	}
	length := int(nL) + int(nH)*256

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}

	switch cmdType[0] {
	case 'U': // ESC ( U nL nH m d (Set motion units)
		if length >= 1 {
			d := data[0]
			if length >= 2 {
				// if m and d both present
				d = data[1]
			}
			if d > 0 {
				s.HorizUnit = 3600 / int(d)
				s.VertUnit = 3600 / int(d)
			}
		}

	case 'V': // ESC ( V nL nH dL dH (Set absolute vertical position)
		if length >= 2 {
			dL := data[0]
			dH := data[1]
			pos := int(dL) + int(dH)*256
			s.Y = pos * (21600 / s.VertUnit)
		}

	case 'v': // ESC ( v nL nH dL dH (Set relative vertical position)
		if length >= 2 {
			dL := data[0]
			dH := data[1]
			offset := int(int16(uint16(dL) | (uint16(dH) << 8)))
			s.Y += offset * (21600 / s.VertUnit)
		}
	}

	return nil
}

// handleBitImage handles ESC * m nL nH [data]
func (s *ParserState) handleBitImage(r io.Reader) error {
	m, err := readByte(r)
	if err != nil {
		return err
	}
	nL, err := readByte(r)
	if err != nil {
		return err
	}
	nH, err := readByte(r)
	if err != nil {
		return err
	}
	cols := int(nL) + int(nH)*256

	// Determine data size
	is24Pin := false
	switch m {
	case 32, 33, 38, 39, 40:
		is24Pin = true
	}

	dataSize := cols
	if is24Pin {
		dataSize = cols * 3
	}

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}

	if is24Pin {
		s.addBitImage24(int(m), cols, data)
	} else {
		s.addBitImage8(int(m), cols, data)
	}

	return nil
}

// addBitImage8 converts 8-dot column-oriented graphics to a Page ImageItem.
func (s *ParserState) addBitImage8(m int, cols int, data []byte) {
	// 8-dot vertical height
	imgH := 8
	imgW := cols

	// Determine DPI resolutions
	hDPI := 60
	switch m {
	case 0:
		hDPI = 60
	case 1, 2:
		hDPI = 120
	case 3:
		hDPI = 240
	case 4:
		hDPI = 80
	case 6:
		hDPI = 90
	}
	vDPI := 72

	// Dimensions in units (1/21600 inch)
	widthUnits := cols * (21600 / hDPI)
	heightUnits := 8 * (21600 / vDPI) // 8 * 300 = 2400 units

	// Allocate row-by-row bitmap
	rowBytes := (cols + 7) / 8
	bitmap := make([]byte, imgH*rowBytes)

	for col := 0; col < cols; col++ {
		b := data[col]
		for row := 0; row < 8; row++ {
			// In Epson 8-dot graphics, bit 7 (MSB) is the top pixel, bit 0 (LSB) is the bottom pixel.
			bit := (b >> (7 - uint(row))) & 1
			if bit != 0 {
				// Set pixel (col, row) in bitmap
				byteIdx := row*rowBytes + (col / 8)
				bitIdx := 7 - uint(col%8)
				bitmap[byteIdx] |= (1 << bitIdx)
			}
		}
	}

	page := s.getPage()
	page.Items = append(page.Items, &ImageItem{
		X:      s.X,
		Y:      s.Y,
		Width:  widthUnits,
		Height: heightUnits,
		ImgW:   imgW,
		ImgH:   imgH,
		Data:   bitmap,
	})

	s.X += widthUnits
}

// addBitImage24 converts 24-dot column-oriented graphics to a Page ImageItem.
func (s *ParserState) addBitImage24(m int, cols int, data []byte) {
	imgH := 24
	imgW := cols

	hDPI := 60
	switch m {
	case 32:
		hDPI = 60
	case 33:
		hDPI = 120
	case 38:
		hDPI = 90
	case 39:
		hDPI = 240
	}
	vDPI := 180

	widthUnits := cols * (21600 / hDPI)
	heightUnits := 24 * (21600 / vDPI) // 24 * 120 = 2880 units

	rowBytes := (cols + 7) / 8
	bitmap := make([]byte, imgH*rowBytes)

	for col := 0; col < cols; col++ {
		b0 := data[3*col]
		b1 := data[3*col+1]
		b2 := data[3*col+2]

		for row := 0; row < 24; row++ {
			var bit byte
			if row < 8 {
				bit = (b0 >> (7 - uint(row))) & 1
			} else if row < 16 {
				bit = (b1 >> (7 - uint(row-8))) & 1
			} else {
				bit = (b2 >> (7 - uint(row-16))) & 1
			}

			if bit != 0 {
				byteIdx := row*rowBytes + (col / 8)
				bitIdx := 7 - uint(col%8)
				bitmap[byteIdx] |= (1 << bitIdx)
			}
		}
	}

	page := s.getPage()
	page.Items = append(page.Items, &ImageItem{
		X:      s.X,
		Y:      s.Y,
		Width:  widthUnits,
		Height: heightUnits,
		ImgW:   imgW,
		ImgH:   imgH,
		Data:   bitmap,
	})

	s.X += widthUnits
}

// handleRasterGraphics parses ESC . c v h m nL nH [data] (Raster Graphics)
func (s *ParserState) handleRasterGraphics(r io.Reader) error {
	c, err := readByte(r)
	if err != nil {
		return err
	}
	v, err := readByte(r)
	if err != nil {
		return err
	}
	h, err := readByte(r)
	if err != nil {
		return err
	}
	m, err := readByte(r)
	if err != nil {
		return err
	}
	nL, err := readByte(r)
	if err != nil {
		return err
	}
	nH, err := readByte(r)
	if err != nil {
		return err
	}
	cols := int(nL) + int(nH)*256

	_ = c // skip color for now

	// Resolution = 3600 / v dpi, 3600 / h dpi
	// Spacing per pixel = 6 * v units (vertical), 6 * h units (horizontal)
	vSpacing := 6 * int(v)
	hSpacing := 6 * int(h)

	imgH := int(m)
	imgW := cols

	rowBytes := (cols + 7) / 8
	dataSize := imgH * rowBytes

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}

	widthUnits := cols * hSpacing
	heightUnits := imgH * vSpacing

	page := s.getPage()
	page.Items = append(page.Items, &ImageItem{
		X:      s.X,
		Y:      s.Y,
		Width:  widthUnits,
		Height: heightUnits,
		ImgW:   imgW,
		ImgH:   imgH,
		Data:   data,
	})

	s.X += widthUnits
	return nil
}

// readByte is a small helper to read a single byte.
func readByte(r io.Reader) (byte, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, err
	}
	return b[0], nil
}
