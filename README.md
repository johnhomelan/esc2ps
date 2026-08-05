# esc2ps: ESC/P2 to PostScript Converter

`esc2ps` is a high-performance Go-based utility that parses captured Epson ESC/P2 dot matrix printer data streams and converts them into standards-compliant PostScript (DSC) files. This allows legacy applications, hardware, or print captures designed for dot matrix printers to be rendered or printed on modern PostScript-compatible printers or converted directly to PDFs.

---

## Features

- **Text and Font Styling**: Supports Roman, SansSerif, and Courier typefaces with attributes like Bold, Italic, Underline, Double-strike, Subscript, Superscript, Double-width, Double-height, and Condensed formatting.
- **Accurate Text Layout**: Emulates physical monospaced grids (10, 12, 15 CPI) as well as proportional spacing with high accuracy.
- **Robust Control Code Support**: Supports form feeds, line feeds, carriage returns, vertical and horizontal tabs, and absolute/relative backspacing/spacing commands.
- **Raster and Bit Image Graphics**: Parses legacy column-oriented dot-matrix graphics (8-pin and 24-pin bit image modes) as well as modern Epson ESC/P2 raster graphics (`ESC .`), converting them on-the-fly to row-oriented bitmaps.
- **Optimized PostScript Output**: Produces clean Document Structuring Conventions (DSC) PostScript. Incorporates performance-optimized prologue macros for exact character-by-character positioning and underline rendering to prevent visual drifting.
- **Standard Stream Pipeline**: Works seamlessly as a standalone CLI tool or inside UNIX pipe environments using stdin and stdout.

---

## Requirements

- **Go**: Version 1.25 or newer is required to build the program.
- **Make**: GNU Make is optional but recommended for building and testing.

---

## Building the Program

You can compile the project using the provided `Makefile` or directly with the Go toolchain.

### Using Make (Recommended)

To build the executable:
```bash
make build
```
This produces a binary named `esc2ps` in the current directory.

### Using standard Go commands

Alternatively, you can build directly using Go:
```bash
go build -o esc2ps .
```

---

## Usage

`esc2ps` provides a simple command-line interface:

```
ESC/P2 to PostScript Converter (esc2ps)
Usage:
  esc2ps [options]

Options:
  -i string
    	Path to captured Epson ESC/P2 binary file (reads from stdin if omitted)
  -o string
    	Path to output PostScript file (writes to stdout if omitted)
  -v	Show version information
```

### Examples

1. **Convert a file directly**:
   ```bash
   ./esc2ps -i epson_capture.bin -o output.ps
   ```

2. **Run as part of a pipeline**:
   ```bash
   cat epson_capture.bin | ./esc2ps > output.ps
   ```

3. **Check the version**:
   ```bash
   ./esc2ps -v
   ```

---

## Running Tests

The project includes unit tests covering font parsing, control codes, graphics decoding, and PostScript rendering.

### Using Make

Run the test suite with:
```bash
make test
```

### Using standard Go commands

```bash
go test -v ./...
```

To clean up all build artifacts, run:
```bash
make clean
```

---

## Developer Documentation: Code Structure & Architecture

This section describes the layout of the code, how the inner components work, and how they coordinate.

### Codebase Organization

The codebase consists of five primary Go files:

1. `go.mod`: The Go module definition.
2. `main.go`: Application entrypoint, command-line interface flag parsing, stream routing, and main execution pipeline.
3. `parser.go`: The parser engine that converts the input binary stream into abstract page models.
4. `ps.go`: The PostScript generator that translates abstract pages into PostScript code.
5. `parser_test.go`: Comprehensive test suite testing text/font styling, 8-dot graphics, 24-dot graphics, raster graphics, and PostScript output.

---

### Internal Workings & Coordination Flow

```
 +------------------+      +-----------------------+      +----------------------------+
 | ESC/P2 Binary In | ---> | parser.go (Parser)    | ---> | ps.go (PostScript Gen)     | ---> PostScript Out
 | (File or stdin)  |      | - State Machine       |      | - Coordinate transformation|
 +------------------+      | - Page/Item Models    |      | - PS Prologue definitions  |
                           +-----------------------+      +----------------------------+
```

#### 1. Unit Space & Conversions (1/21600 Inch Grid)
To handle the fine horizontal and vertical positioning of Epson dot-matrix printers, the parser operates internally in **Epson unit space**, where the base resolution is **21600 units per inch** (resulting in clean multiples of common DPI values like 60, 120, 180, and 360).

PostScript operates in standard **points**, where **1 point = 1/72 inch**. To bridge these coordinate systems:
- Let $u$ be the coordinate in Epson unit space ($1/21600$ inch).
- Let $p$ be the coordinate in PostScript points ($1/72$ inch).
- The conversion factor is:
  $$\frac{21600}{72} = 300\text{ units per PostScript point}$$
- Therefore, in `ps.go`, the absolute coordinates are converted using:
  $$p = \frac{u}{300.0}$$

#### 2. The Parsing Engine (`parser.go`)
The core parsing is executed inside `Parse(io.Reader)`, which processes the incoming byte stream sequentially. 

- **State Machine (`ParserState`)**: Tracks printer configurations such as typeface, line spacing, margins, active character pitch (condensed, proportional, 10/12/15 CPI), absolute and relative cursor coordinates ($X$ and $Y$ in unit space), and character modification flags (bold, italic, underline, superscript/subscript).
- **Page Laziness**: When the parser encounters printable characters, it lazily initializes pages and items. If a vertical shift (such as form feeds or text overflow) crosses page boundaries, the state's `CurrentPage` is set to `nil`, pushing subsequent operations to a new page.
- **Text Coalescing**: To generate compact and efficient outputs, the parser coalesces sequential text characters into a single `TextItem` if they share the same line, styling properties, and matching horizontal grid slots. Gaps are automatically backfilled with space characters.
- **Graphics Handling**: 
  - Dot matrix printheads print vertically column-by-column (e.g. 8-dot or 24-dot slices). `addBitImage8` and `addBitImage24` handle these sequences, mapping the incoming vertical printhead scanlines into standard row-by-row monochrome bitmap buffers suitable for traditional modern display.
  - Modern raster graphics (`ESC .`) are parsed by `handleRasterGraphics`, preserving the exact dimensions and compression settings specified by the driver.

#### 3. PostScript Code Generation (`ps.go`)
Once parsing completes, `GeneratePostScript(pages, io.Writer)` writes standard DSC-compliant PostScript.

- **Coordinate Mapping**: Epson printer layout starts at the top-left $(0,0)$ and goes down. PostScript starts at the bottom-left $(0,0)$ and goes up. In `ps.go`, the $Y$ coordinate is converted via:
  $$Y_{\text{PostScript}} = \frac{\text{PageHeight} - Y_{\text{Epson}}}{300.0}$$
- **Prologue Procedures**: To optimize file size and rendering accuracy, the output defines custom procedures in its header:
  - `/S`: Standardizes character-by-character drawing. It takes $(X, Y)$ absolute starting coordinates, a text string, and an exact character-width advance in points. It loops through the characters, placing each precisely along the monospaced grid, eliminating character drifting.
  - `/draw_underline`: Draws lines dynamically adjusted to the baseline and width of the preceding text.
- **Image Rendering**: Image items (`ImageItem`) are drawn by positioning their bounding box, translating coordinates, and filtering hex encoded bitmap patterns through `/ASCIIHexDecode filter image` directly into the stream.

---

## Extending the Project

Developers wishing to extend `esc2ps` can focus on:
1. **Adding Support for New Escape Commands**: Update the `handleESC` method inside `parser.go` to match commands documented in Epson’s ESC/P2 reference manuals.
2. **Color Printing**: Although colors are currently parsed but skipped in `handleESC` (e.g., `ESC r n`), support can be added by introducing color state tracking to `ParserState` and generating corresponding PostScript color operations (such as `setrgbcolor`).
3. **Advanced Test Assertions**: Extend `parser_test.go` to cover edge-case escape sequence combinations, ensuring no regressions are introduced into the parser state machine.
