package main

import (
	"bytes"
	"fmt"
	"io"
)

// getRGBColor maps Epson color ribbon numbers to RGB float64 components.
func getRGBColor(color int) (r, g, b float64) {
	switch color {
	case 1: // Magenta
		return 1.0, 0.0, 1.0
	case 2: // Cyan
		return 0.0, 1.0, 1.0
	case 3: // Violet
		return 0.5, 0.0, 1.0
	case 4: // Yellow
		return 1.0, 1.0, 0.0
	case 5: // Orange
		return 1.0, 0.5, 0.0
	case 6: // Green
		return 0.0, 0.8, 0.0
	default: // 0: Black, and other invalid indices
		return 0.0, 0.0, 0.0
	}
}

// escapePSString escapes parentheses and backslashes, and converts non-printable characters to octal escapes.
func escapePSString(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\', '(', ')':
			result = append(result, '\\', ch)
		default:
			if ch < 32 || ch > 126 {
				result = append(result, []byte(fmt.Sprintf("\\%03o", ch))...)
			} else {
				result = append(result, ch)
			}
		}
	}
	return string(result)
}

// basePSFonts lists the Adobe base-14 fonts used by getPSFontName, each of
// which GeneratePostScript re-encodes to ISOLatin1Encoding in the prologue.
var basePSFonts = []string{
	"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
	"Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
	"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
}

// getPSFontName maps the Epson font settings to standard PostScript core fonts.
// Returned names (other than EpsonUserFont) carry a "-Latin1" suffix, referring
// to the ISOLatin1Encoding-reencoded variants GeneratePostScript defines in the
// prologue (see basePSFonts), so Latin-1 byte values render correctly.
func getPSFontName(font FontInfo) string {
	if font.UserFont {
		return "EpsonUserFont"
	}
	switch font.Typeface {
	case "Roman":
		if font.Bold && font.Italic {
			return "Times-BoldItalic-Latin1"
		} else if font.Bold {
			return "Times-Bold-Latin1"
		} else if font.Italic {
			return "Times-Italic-Latin1"
		} else {
			return "Times-Roman-Latin1"
		}
	case "SansSerif":
		if font.Bold && font.Italic {
			return "Helvetica-BoldOblique-Latin1"
		} else if font.Bold {
			return "Helvetica-Bold-Latin1"
		} else if font.Italic {
			return "Helvetica-Oblique-Latin1"
		} else {
			return "Helvetica-Latin1"
		}
	default: // Courier
		if font.Bold && font.Italic {
			return "Courier-BoldOblique-Latin1"
		} else if font.Bold {
			return "Courier-Bold-Latin1"
		} else if font.Italic {
			return "Courier-Oblique-Latin1"
		} else {
			return "Courier-Latin1"
		}
	}
}

// GeneratePostScript converts the parsed pages into a PostScript file and writes it to w.
func GeneratePostScript(pages []*Page, w io.Writer) error {
	// Standard Epson Top-of-Form margin is 8.5 mm (approx 24 PostScript points)
	const tofMarginPoints = 24.0

	// Write standard EPS/PS DSC headers
	io.WriteString(w, "%!PS-Adobe-3.0\n")
	io.WriteString(w, "%%Creator: esc2ps (Go)\n")
	io.WriteString(w, "%%Title: ESC/P2 Dot Matrix Document\n")
	fmt.Fprintf(w, "%%%%Pages: %d\n", len(pages))
	io.WriteString(w, "%%PageOrder: Ascend\n")
	io.WriteString(w, "%%DocumentMedia: Default 612 792 0 () ()\n")
	io.WriteString(w, "%%EndComments\n")
	fmt.Fprintln(w, "")

	// Write prologue with helpers
	io.WriteString(w, "% --- Prologue ---\n")
	// S: draws a string with character-by-character positioning
	fmt.Fprintln(w, "/S { % x y string char_w")
	fmt.Fprintln(w, "    /w exch def")
	fmt.Fprintln(w, "    /str exch def")
	fmt.Fprintln(w, "    /cy exch def")
	fmt.Fprintln(w, "    /cx exch def")
	fmt.Fprintln(w, "    0 1 str length 1 sub {")
	fmt.Fprintln(w, "        /i exch def")
	fmt.Fprintln(w, "        cx i w mul add cy moveto")
	fmt.Fprintln(w, "        str i 1 getinterval show")
	fmt.Fprintln(w, "    } for")
	fmt.Fprintln(w, "} def")
	fmt.Fprintln(w, "")

	// draw_underline: draws underline matching font size
	fmt.Fprintln(w, "/draw_underline { % x y width size")
	fmt.Fprintln(w, "    /sz exch def")
	fmt.Fprintln(w, "    /w exch def")
	fmt.Fprintln(w, "    /y exch def")
	fmt.Fprintln(w, "    /x exch def")
	fmt.Fprintln(w, "    gsave")
	fmt.Fprintln(w, "    newpath")
	fmt.Fprintln(w, "    x y sz 0.12 mul sub moveto")
	fmt.Fprintln(w, "    w 0 rlineto")
	fmt.Fprintln(w, "    sz 0.06 mul setlinewidth")
	fmt.Fprintln(w, "    stroke")
	fmt.Fprintln(w, "    grestore")
	fmt.Fprintln(w, "} def")
	fmt.Fprintln(w, "")

	// Re-encode the base-14 fonts to ISOLatin1Encoding so that Latin-1 byte
	// values (e.g. 0xA3 for the international-character-set substitutions
	// applied by ESC R) render as the correct accented/national glyphs
	// instead of whatever StandardEncoding happens to have at that slot.
	io.WriteString(w, "% --- Latin-1 re-encoded base fonts ---\n")
	for _, base := range basePSFonts {
		fmt.Fprintf(w, "/%s findfont\n", base)
		fmt.Fprintln(w, "dup length dict begin")
		fmt.Fprintln(w, "  {1 index /FID ne {def} {pop pop} ifelse} forall")
		fmt.Fprintln(w, "  /Encoding ISOLatin1Encoding def")
		fmt.Fprintln(w, "  currentdict")
		fmt.Fprintln(w, "end")
		fmt.Fprintf(w, "/%s-Latin1 exch definefont pop\n", base)
	}
	fmt.Fprintln(w, "")

	// Check if we need to define the user-defined font
	var hasUserFont bool
	var userChars map[byte]*UserChar
	for _, page := range pages {
		if len(page.UserChars) > 0 {
			hasUserFont = true
			userChars = page.UserChars
			break
		}
	}

	if hasUserFont {
		io.WriteString(w, "% --- EpsonUserFont Definition (Type 3) ---\n")
		io.WriteString(w, "8 dict begin\n")
		io.WriteString(w, "  /FontType 3 def\n")
		io.WriteString(w, "  /FontMatrix [ 0.041667 0 0 0.041667 0 0 ] def\n")
		io.WriteString(w, "  /FontBBox [ 0 -6 24 18 ] def\n")
		io.WriteString(w, "  /Encoding 256 array def\n")
		io.WriteString(w, "  0 1 255 { Encoding exch /.notdef put } for\n")
		
		for code := range userChars {
			fmt.Fprintf(w, "  Encoding %d /c%d put\n", code, code)
		}
		
		io.WriteString(w, "  /CharStrings 256 dict def\n")
		io.WriteString(w, "  CharStrings begin\n")
		io.WriteString(w, "    /.notdef { 0 0 setcharwidth } def\n")
		
		for code, char := range userChars {
			var hexBuf bytes.Buffer
			for _, b := range char.Bitmap {
				hexBuf.WriteString(fmt.Sprintf("%02X", b))
			}
			fmt.Fprintf(w, "    /c%d {\n", code)
			fmt.Fprintf(w, "      %d 0 0 -6 %d 18 setcachedevice\n", char.Width, char.Width)
			fmt.Fprintf(w, "      gsave\n")
			fmt.Fprintf(w, "      0 -6 translate\n")
			fmt.Fprintf(w, "      %d 24 true [ %d 0 0 -24 0 24 ] { <%s> } imagemask\n", char.Width, char.Width, hexBuf.String())
			fmt.Fprintf(w, "      grestore\n")
			fmt.Fprintf(w, "    } def\n")
		}
		io.WriteString(w, "  end\n")
		
		io.WriteString(w, "  /BuildChar {\n")
		io.WriteString(w, "    2 copy exch /Encoding get exch get\n")
		io.WriteString(w, "    3 index /CharStrings get exch get\n")
		io.WriteString(w, "    3 1 roll pop pop\n")
		io.WriteString(w, "    exec\n")
		io.WriteString(w, "  } def\n")
		io.WriteString(w, "  currentdict\n")
		io.WriteString(w, "end\n")
		io.WriteString(w, "/EpsonUserFont exch definefont pop\n\n")
	}

	io.WriteString(w, "%%EndPrologue\n")
	fmt.Fprintln(w, "")

	// Render pages
	for pageIdx, page := range pages {
		fmt.Fprintf(w, "%%%%Page: %d %d\n", pageIdx+1, pageIdx+1)
		fmt.Fprintf(w, "%%%%BeginPageSetup\n")
		fmt.Fprintf(w, "%%%%PaperSize: Letter\n")
		fmt.Fprintf(w, "%%%%EndPageSetup\n")
		fmt.Fprintln(w, "")

		// Initialize state tracking to avoid redundant PostScript instructions
		var currentFontName string
		var currentFontSize float64
		var currentFontMatrix string
		currentColor := 0

		for _, item := range page.Items {
			switch it := item.(type) {
			case *TextItem:
				// Map coordinates: Epson top-left (0,0) to PS bottom-left (0,0)
				ptX := float64(it.X) / 300.0
				ptY := (float64(page.Height-it.Y) / 300.0) - tofMarginPoints

				// Handle sub/superscript adjustments
				fontSize := it.Font.Size
				if it.Font.SuperSub == 1 {
					// Superscript: shrink and shift up
					fontSize *= 0.6
					ptY += it.Font.Size * 0.3
				} else if it.Font.SuperSub == -1 {
					// Subscript: shrink and shift down
					fontSize *= 0.6
					ptY -= it.Font.Size * 0.15
				}

				// Set color if changed
				if it.Font.Color != currentColor {
					r, g, b := getRGBColor(it.Font.Color)
					fmt.Fprintf(w, "%f %f %f setrgbcolor\n", r, g, b)
					currentColor = it.Font.Color
				}

				// Font styling
				fontName := getPSFontName(it.Font)

				// Check if we need to scale font width or height
				var fontMatrix string
				hScale := 1.0
				vScale := 1.0
				if it.Font.DoubleWidth {
					hScale = 2.0
				}
				if it.Font.DoubleHeight {
					vScale = 2.0
				}

				if hScale != 1.0 || vScale != 1.0 {
					fontMatrix = fmt.Sprintf("[ %f 0 0 %f 0 0 ]", fontSize*hScale, fontSize*vScale)
				}

				// Set font if changed
				if fontMatrix != "" {
					if fontMatrix != currentFontMatrix || fontName != currentFontName {
						fmt.Fprintf(w, "/%s findfont %s makefont setfont\n", fontName, fontMatrix)
						currentFontName = fontName
						currentFontMatrix = fontMatrix
						currentFontSize = 0
					}
				} else {
					if fontSize != currentFontSize || fontName != currentFontName || currentFontMatrix != "" {
						fmt.Fprintf(w, "/%s findfont %f scalefont setfont\n", fontName, fontSize)
						currentFontName = fontName
						currentFontSize = fontSize
						currentFontMatrix = ""
					}
				}

				// Calculate character width and print string
				charWidthUnits := float64(it.Font.Pitch) // default dummy or computed
				_ = charWidthUnits

				// Character advance in PS points
				// In parser, charWidth() returns the advance in 21600 units
				// We need to pass the character advance to the S function in PS points (divide by 300)
				// Wait! How did we calculate the character advance in parser?
				// To get the exact spacing, let's look at the active character width in points:
				// We can re-calculate or store it. Let's calculate from the font settings.
				var baseWidth int
				if it.Font.Pitch == -1 {
					// Proportional
					baseWidth = int((fontSize / 72.0) * 21600.0 * 0.5)
				} else {
					pitch := it.Font.Pitch
					if it.Font.Condensed {
						switch pitch {
						case 10:
							baseWidth = 21600 / 17
						case 12:
							baseWidth = 21600 / 20
						case 15:
							baseWidth = 21600 / 25
						default:
							baseWidth = 21600 / 17
						}
					} else {
						switch pitch {
						case 10:
							baseWidth = 21600 / 10
						case 12:
							baseWidth = 21600 / 12
						case 15:
							baseWidth = 21600 / 15
						default:
							baseWidth = 21600 / 10
						}
					}
				}

				if it.Font.DoubleWidth {
					baseWidth *= 2
				}
				charWidthPoints := float64(baseWidth) / 300.0

				escapedText := escapePSString(it.Text)
				if it.Font.Pitch == -1 {
					// Proportional font: let PS space characters naturally
					fmt.Fprintf(w, "%f %f moveto (%s) show\n", ptX, ptY, escapedText)
					if it.Font.Underline {
						// Underline with natural width
						fmt.Fprintf(w, "%f %f (%s) stringwidth pop %f draw_underline\n", ptX, ptY, escapedText, fontSize)
					}
				} else {
					// Monospaced / Gridded font: use S function to force exact grid spacing
					fmt.Fprintf(w, "%f %f (%s) %f S\n", ptX, ptY, escapedText, charWidthPoints)
					if it.Font.Underline {
						// Underline with gridded width
						totalW := charWidthPoints * float64(len(it.Text))
						fmt.Fprintf(w, "%f %f %f %f draw_underline\n", ptX, ptY, totalW, fontSize)
					}
				}

			case *ImageItem:
				// Draw raster image
				// Top-left coordinate is (it.X, it.Y)
				// We need to place it such that the top-left of the image is at that point on the page.
				// In PS, coordinates are bottom-left.
				// Bottom-left of image on page:
				ptX := float64(it.X) / 300.0
				ptYTop := (float64(page.Height-it.Y) / 300.0) - tofMarginPoints
				hUnitsPoints := float64(it.Height) / 300.0
				wUnitsPoints := float64(it.Width) / 300.0
				ptYBottom := ptYTop - hUnitsPoints

				fmt.Fprintln(w, "gsave")
				fmt.Fprintf(w, "%f %f translate\n", ptX, ptYBottom)
				fmt.Fprintf(w, "%f %f scale\n", wUnitsPoints, hUnitsPoints)
				fmt.Fprintf(w, "%d %d 1 [ %d 0 0 %d neg 0 %d ] currentfile /ASCIIHexDecode filter image\n",
					it.ImgW, it.ImgH, it.ImgW, it.ImgH, it.ImgH)

				// Write image hex data
				for i := 0; i < len(it.Data); i++ {
					fmt.Fprintf(w, "%02X", it.Data[i])
					if (i+1)%40 == 0 {
						fmt.Fprintln(w, "") // newline every 40 bytes (80 hex chars) for neatness
					}
				}
				fmt.Fprintln(w, ">") // terminate ASCIIHexDecode
				fmt.Fprintln(w, "grestore")
			}
		}

		fmt.Fprintln(w, "showpage")
		fmt.Fprintln(w, "")
	}

	io.WriteString(w, "%%EOF\n")
	return nil
}
