package todoui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// This file renders an idea's mind map as a real left-to-right diagram: every
// node is a rounded box, and parents fan out to their children through a shared
// connector "bus" with proper box-drawing junctions — closer to a commercial
// mind map than an indented outline.

// Box-drawing edge bits. A line cell is the OR of the directions it connects.
const (
	mbUp    uint8 = 1
	mbDown  uint8 = 2
	mbLeft  uint8 = 4
	mbRight uint8 = 8
)

// boxRune maps a set of edge bits to its light box-drawing glyph.
var boxRune = map[uint8]rune{
	0:                                ' ',
	mbUp:                             '│',
	mbDown:                           '│',
	mbUp | mbDown:                    '│',
	mbLeft:                           '─',
	mbRight:                          '─',
	mbLeft | mbRight:                 '─',
	mbDown | mbRight:                 '┌',
	mbDown | mbLeft:                  '┐',
	mbUp | mbRight:                   '└',
	mbUp | mbLeft:                    '┘',
	mbUp | mbDown | mbRight:          '├',
	mbUp | mbDown | mbLeft:           '┤',
	mbDown | mbLeft | mbRight:        '┬',
	mbUp | mbLeft | mbRight:          '┴',
	mbUp | mbDown | mbLeft | mbRight: '┼',
}

// mindPalette holds the mindColorCount node colours cycled by c/C/b/B. Index 0
// (default) is handled separately, so palette[i] backs colour index i+1.
var mindPalette = []lipgloss.Color{
	lipgloss.Color("#ef4444"), // red
	lipgloss.Color("#f97316"), // orange
	lipgloss.Color("#eab308"), // amber
	lipgloss.Color("#22c55e"), // green
	lipgloss.Color("#14b8a6"), // teal
	lipgloss.Color("#3b82f6"), // blue
	lipgloss.Color("#6366f1"), // indigo
	lipgloss.Color("#a855f7"), // purple
	lipgloss.Color("#ec4899"), // pink
	lipgloss.Color("#94a3b8"), // slate
}

// mindInk is a near-black foreground used for text/borders on a filled box, so
// the label stays readable on any background colour.
var mindInk = lipgloss.Color("#0b0f19")

// mindBlurColor is the faint grey the whole map is flattened to while zoomed, so
// the centered popup reads as in-focus and the map as a blurred backdrop.
var mindBlurColor = lipgloss.Color("#3a3f4b")

// paletteColor returns the colour for a 1-based index, or "" for 0 (default).
func paletteColor(idx int) lipgloss.Color {
	if idx <= 0 || idx > len(mindPalette) {
		return ""
	}
	return mindPalette[idx-1]
}

// mindBox is one laid-out node in the diagram.
type mindBox struct {
	node     *MindNode // nil for the root idea
	isRoot   bool
	selected bool
	editing  bool
	depth    int
	text     string
	outline  int // outline colour index
	bg       int // background colour index
	w, h     int // box size in cells (h is always 3)
	x, y     int // top-left cell
	cy       int // vertical centre row
	kids     []*mindBox
}

const (
	mmMaxText = 26 // node text is truncated to this many runes
	mmHGap    = 6  // cells between columns (room for the connector bus)
	mmVGap    = 1  // blank rows between sibling subtrees
	mmBoxH    = 3
)

// mmTruncate flattens whitespace and caps the text to max runes with an ellipsis.
func mmTruncate(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// nodeBoxText is the label shown inside a node's box. Markers stay ASCII so each
// glyph is exactly one cell wide and the canvas grid never drifts.
func nodeBoxText(n *MindNode) string {
	t := n.Text
	if t == "" {
		t = "·"
	}
	if n.IsTask {
		box := "[ ] "
		if n.Done {
			box = "[x] "
		}
		t = box + t
	}
	if n.Collapsed && len(n.Children) > 0 {
		t = fmt.Sprintf("%s (+%d)", t, len(n.Children))
	}
	return t
}

// buildMindBoxes lays out the visible tree of the current idea into boxes with
// absolute cell coordinates, and reports the total canvas size.
func (m model) buildMindBoxes() (root *mindBox, all []*mindBox, w, h int) {
	if m.mindIdea < 0 || m.mindIdea >= len(m.ideas) {
		return nil, nil, 0, 0
	}
	idea := m.ideas[m.mindIdea]

	// Identify the selected / edited node from the navigation rows.
	rows := m.mindRows()
	var selNode, editNode *MindNode
	selRoot, editRoot := false, false
	if m.mindCursor >= 0 && m.mindCursor < len(rows) {
		if r := rows[m.mindCursor]; r.isRoot {
			selRoot = true
		} else {
			selNode = r.node
		}
	}
	if m.mode == modeMindEdit {
		if m.mindEditNode == nil {
			editRoot = true
		} else {
			editNode = m.mindEditNode
		}
	}
	liveEdit := m.input.Value() + "▌"

	var build func(node *MindNode, isRoot bool, depth int) *mindBox
	build = func(node *MindNode, isRoot bool, depth int) *mindBox {
		b := &mindBox{depth: depth, isRoot: isRoot, node: node, h: mmBoxH}
		switch {
		case isRoot:
			b.selected = selRoot
			b.editing = editRoot
			b.text = idea.Text
			b.outline = idea.Color
			b.bg = idea.BG
		default:
			b.selected = node == selNode
			b.editing = node == editNode
			b.text = nodeBoxText(node)
			b.outline = node.Color
			b.bg = node.BG
		}
		if b.editing {
			b.text = liveEdit
		} else {
			b.text = mmTruncate(b.text, mmMaxText)
		}
		b.w = utf8.RuneCountInString(b.text) + 4 // 2 borders + 2 padding
		if b.w < 5 {
			b.w = 5
		}
		all = append(all, b)

		var children []*MindNode
		if isRoot {
			children = idea.Children
		} else if !node.Collapsed {
			children = node.Children
		}
		for _, c := range children {
			b.kids = append(b.kids, build(c, false, depth+1))
		}
		return b
	}
	root = build(nil, true, 0)

	// Columns: every node at the same depth shares a left edge.
	maxW := map[int]int{}
	maxDepth := 0
	for _, b := range all {
		if b.w > maxW[b.depth] {
			maxW[b.depth] = b.w
		}
		if b.depth > maxDepth {
			maxDepth = b.depth
		}
	}
	colX := make([]int, maxDepth+1)
	x := 0
	for d := 0; d <= maxDepth; d++ {
		colX[d] = x
		x += maxW[d] + mmHGap
	}
	for _, b := range all {
		b.x = colX[b.depth]
	}

	// Rows: pack leaves top-to-bottom; centre each parent on its children.
	nextY := 0
	var place func(b *mindBox)
	place = func(b *mindBox) {
		if len(b.kids) == 0 {
			b.y = nextY
			b.cy = b.y + b.h/2
			nextY += b.h + mmVGap
			return
		}
		for _, k := range b.kids {
			place(k)
		}
		b.cy = (b.kids[0].cy + b.kids[len(b.kids)-1].cy) / 2
		b.y = b.cy - b.h/2
	}
	place(root)

	for _, b := range all {
		if b.y+b.h > h {
			h = b.y + b.h
		}
		if b.x+b.w > w {
			w = b.x + b.w
		}
	}
	return root, all, w, h
}

// cell holds everything needed to render one canvas position.
type cell struct {
	ch    rune
	bits  uint8
	fixed bool // rune is final (rounded corner) — skip bit→rune pass
	fg    lipgloss.Color
	bg    lipgloss.Color
	bold  bool
	ul    bool // underline (used to mark the selected node)
	dirty bool // anything was painted here
}

// mindCanvas is a 2-D grid of cells.
type mindCanvas struct {
	w, h int
	g    [][]cell
}

func newMindCanvas(w, h int) *mindCanvas {
	c := &mindCanvas{w: w, h: h, g: make([][]cell, h)}
	for y := 0; y < h; y++ {
		c.g[y] = make([]cell, w)
		for x := 0; x < w; x++ {
			c.g[y][x].ch = ' '
		}
	}
	return c
}

func (c *mindCanvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

// line adds connector/border bits to a cell with a foreground colour.
func (c *mindCanvas) line(x, y int, bit uint8, fg lipgloss.Color, bold bool) {
	if !c.in(x, y) {
		return
	}
	p := &c.g[y][x]
	p.bits |= bit
	p.fg = fg
	p.bold = bold
	p.dirty = true
}

func (c *mindCanvas) corner(x, y int, r rune, fg lipgloss.Color, bold bool) {
	if !c.in(x, y) {
		return
	}
	p := &c.g[y][x]
	p.ch = r
	p.fixed = true
	p.fg = fg
	p.bold = bold
	p.dirty = true
}

func (c *mindCanvas) put(x, y int, r rune, fg lipgloss.Color, bold, ul bool) {
	if !c.in(x, y) {
		return
	}
	p := &c.g[y][x]
	p.ch = r
	p.fg = fg
	p.bold = bold
	p.ul = ul
	p.dirty = true
}

func (c *mindCanvas) fill(x, y int, bg lipgloss.Color) {
	if !c.in(x, y) || bg == "" {
		return
	}
	c.g[y][x].bg = bg
	c.g[y][x].dirty = true
}

// drawBox paints a rounded rectangle with left-aligned text for one node,
// resolving its outline / background / selection colours.
func (c *mindCanvas) drawBox(b *mindBox) {
	x0, y0 := b.x, b.y
	x1, y1 := b.x+b.w-1, b.y+b.h-1

	outCol := paletteColor(b.outline)
	bgCol := paletteColor(b.bg)

	// The border always shows the node's own colour, so c/C are visible
	// immediately. Selection is indicated by underlining the text instead.
	var borderFg, textFg lipgloss.Color
	switch {
	case outCol != "":
		borderFg = outCol
	case bgCol != "":
		borderFg = mindInk
	default:
		borderFg = subColor
	}
	switch {
	case bgCol != "":
		textFg = mindInk
	case b.isRoot:
		textFg = brightColor
	default:
		textFg = textColor
	}
	borderBold := b.selected || outCol != ""
	textBold := b.isRoot || b.selected

	// Background fill across the whole box first.
	if bgCol != "" {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				c.fill(x, y, bgCol)
			}
		}
	}

	// Rounded corners + edges.
	c.corner(x0, y0, '╭', borderFg, borderBold)
	c.corner(x1, y0, '╮', borderFg, borderBold)
	c.corner(x0, y1, '╰', borderFg, borderBold)
	c.corner(x1, y1, '╯', borderFg, borderBold)
	for x := x0 + 1; x < x1; x++ {
		c.line(x, y0, mbLeft|mbRight, borderFg, borderBold)
		c.line(x, y1, mbLeft|mbRight, borderFg, borderBold)
	}
	c.line(x0, b.cy, mbUp|mbDown, borderFg, borderBold)
	c.line(x1, b.cy, mbUp|mbDown, borderFg, borderBold)

	// Text (no per-character underline — selection is shown under the box).
	tx := x0 + 2
	for _, r := range b.text {
		if tx >= x1 {
			break
		}
		c.put(tx, b.cy, r, textFg, textBold, false)
		tx++
	}

	// Selection cue: underline the box's bottom border row, so the line sits
	// under the whole box rather than under the words.
	if b.selected {
		for x := x0; x <= x1; x++ {
			if c.in(x, y1) {
				c.g[y1][x].ul = true
			}
		}
	}
}

// connect draws the connector bus from a parent to all of its children.
func (c *mindCanvas) connect(p *mindBox) {
	if len(p.kids) == 0 {
		return
	}
	childLeft := p.kids[0].x
	busX := childLeft - mmHGap/2
	if busX <= p.x+p.w-1 {
		busX = p.x + p.w
	}
	pRight := p.x + p.w - 1

	// Parent → bus.
	c.line(pRight, p.cy, mbRight, subColor, false)
	for x := pRight + 1; x < busX; x++ {
		c.line(x, p.cy, mbLeft|mbRight, subColor, false)
	}
	c.line(busX, p.cy, mbLeft, subColor, false)

	// Vertical bus spanning the children's centres.
	top, bot := p.kids[0].cy, p.kids[len(p.kids)-1].cy
	for y := top; y <= bot; y++ {
		if y > top {
			c.line(busX, y, mbUp, subColor, false)
		}
		if y < bot {
			c.line(busX, y, mbDown, subColor, false)
		}
	}

	// Bus → each child.
	for _, k := range p.kids {
		c.line(busX, k.cy, mbRight, subColor, false)
		for x := busX + 1; x < k.x; x++ {
			c.line(x, k.cy, mbLeft|mbRight, subColor, false)
		}
		c.line(k.x, k.cy, mbLeft, subColor, false)
	}
}

// mindMapView renders the whole mind-map screen: title, the boxed diagram (with
// a viewport that follows the cursor) and the key-hint footer.
func (m model) mindMapView(header string) string {
	yellow := lipgloss.NewStyle().Foreground(dueColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)

	root, all, cw, ch := m.buildMindBoxes()

	accentKey := lipgloss.NewStyle().Foreground(brandRed).Bold(true)

	// Bound-project state drives both the title and the T hint.
	bound := ""
	if m.mindIdea >= 0 && m.mindIdea < len(m.ideas) {
		bound = m.ideas[m.mindIdea].ProjectName
	}
	title := yellow.Render("🗺  Mind map")
	if bound != "" {
		title += lipgloss.NewStyle().Foreground(projectColor).Render("  → " + bound)
	}

	tHint := "T→project"
	if bound != "" {
		tHint = "T→" + bound + " · U unbind"
	}
	var footer string
	switch {
	case m.mode == modeMindEdit:
		footer = dim.Render("type · enter save · esc cancel")
	case m.mindZoom:
		// Zoom overlay: the selected node's full text floats over the map, which
		// stays navigable underneath; z/esc close it.
		footer = accentKey.Render("🔍 zoom") + dim.Render(
			" · ↑↓/jk·←→/hl navigate · e edit · tab child · z/esc close · ") +
			accentKey.Render("H") + dim.Render(" help")
	default:
		// "H help" is accented first so it stays visible, matching the task view.
		footer = accentKey.Render("H help") + dim.Render(
			" · ↑↓/jk siblings · ←→/hl parent·child · r root · tab child · enter sibling · e edit · z zoom · t task · "+
				tHint+" · x done · d del · c/C colour · v/V fill · s sync · b back")
	}

	viewH := m.height - lipgloss.Height(header) - 4 // title + gap + footer + margin
	if viewH < 3 {
		viewH = 3
	}
	viewW := m.width
	if viewW < 10 {
		viewW = 10
	}

	if root == nil {
		return lipgloss.JoinVertical(lipgloss.Left, header, title, "", dim.Render("  (empty)"), "", footer)
	}

	cv := newMindCanvas(cw, ch)
	for _, b := range all { // connectors first; boxes paint over the attach cells
		cv.connect(b)
	}
	for _, b := range all {
		cv.drawBox(b)
	}
	// Resolve line cells from their accumulated bits.
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			p := &cv.g[y][x]
			if !p.fixed && p.bits != 0 {
				p.ch = boxRune[p.bits]
			}
		}
	}

	// Viewport follows the cursor / edited box.
	var focus *mindBox
	for _, b := range all {
		if b.selected {
			focus = b
			break
		}
	}
	if focus == nil {
		focus = root
	}
	clamp := func(v, max int) int {
		if v < 0 {
			return 0
		}
		if v > max {
			return max
		}
		return v
	}
	scrollX := clamp(focus.x+focus.w/2-viewW/2, max0(cw-viewW))
	scrollY := clamp(focus.cy-viewH/2, max0(ch-viewH))

	// While zoomed, dim the whole map to a faint monochrome so the centered
	// popup stands out — the terminal equivalent of blurring the background.
	zoomDim := m.mindZoom && m.mode != modeMindEdit

	// Emit visible rows, grouping consecutive cells that share styling so colour
	// is applied per run rather than per cell.
	var lines []string
	for y := scrollY; y < scrollY+viewH && y < ch; y++ {
		var b strings.Builder
		var run strings.Builder
		var rFg, rBg lipgloss.Color
		var rBold, rUL bool
		flush := func() {
			if run.Len() == 0 {
				return
			}
			st := lipgloss.NewStyle()
			if rFg != "" {
				st = st.Foreground(rFg)
			}
			if rBg != "" {
				st = st.Background(rBg)
			}
			if rBold {
				st = st.Bold(true)
			}
			if rUL {
				st = st.Underline(true)
			}
			b.WriteString(st.Render(run.String()))
			run.Reset()
		}
		for x := scrollX; x < scrollX+viewW && x < cw; x++ {
			p := cv.g[y][x]
			fg, bg, bold, ul := p.fg, p.bg, p.bold, p.ul
			if !p.dirty {
				fg, bg, bold, ul = "", "", false, false
			}
			if zoomDim && p.dirty {
				// Flatten every painted cell to one faint colour (no fill,
				// no bold, no underline) for the "blurred background" look.
				fg, bg, bold, ul = mindBlurColor, "", false, false
			}
			if run.Len() > 0 && (fg != rFg || bg != rBg || bold != rBold || ul != rUL) {
				flush()
			}
			rFg, rBg, rBold, rUL = fg, bg, bold, ul
			run.WriteRune(p.ch)
		}
		flush()
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}

	// Scroll affordances.
	var sh []string
	if scrollY > 0 {
		sh = append(sh, "↑")
	}
	if scrollY+viewH < ch {
		sh = append(sh, "↓")
	}
	if scrollX > 0 {
		sh = append(sh, "←")
	}
	if scrollX+viewW < cw {
		sh = append(sh, "→")
	}
	titleLine := title
	if len(sh) > 0 {
		titleLine = title + dim.Render("   more: "+strings.Join(sh, " "))
	}

	// Zoom overlay: float the selected node's full text, centered over the
	// (dimmed) map. Hidden while editing so it doesn't cover the live input.
	if zoomDim {
		ov := m.mindZoomOverlay(viewW)
		for len(lines) < viewH { // pad the backdrop so the popup centers in it
			lines = append(lines, "")
		}
		ovW := 0
		if len(ov) > 0 {
			ovW = ansi.StringWidth(ov[0])
		}
		top := max0((viewH - len(ov)) / 2)
		left := max0((viewW - ovW) / 2)
		lines = overlayAt(lines, ov, top, left)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.JoinVertical(lipgloss.Left, header, titleLine, "", body, "", footer)
}

// mindZoomOverlay renders a floating popup with the selected node's full,
// untruncated text — the boxes in the map cap labels to mmMaxText runes. It
// mirrors the node's own colours so the popup reads as "the same node, complete".
func (m model) mindZoomOverlay(maxW int) []string {
	rows := m.mindRows()
	if len(rows) == 0 {
		return nil
	}
	i := m.mindCursor
	if i < 0 {
		i = 0
	}
	if i >= len(rows) {
		i = len(rows) - 1
	}
	cur := rows[i]

	var full string
	var outline, bg int
	if cur.isRoot {
		idea := m.ideas[m.mindIdea]
		full, outline, bg = idea.Text, idea.Color, idea.BG
	} else {
		full, outline, bg = nodeBoxText(cur.node), cur.node.Color, cur.node.BG
	}
	if strings.TrimSpace(full) == "" {
		full = "·"
	}

	wrap := maxW - 8
	if wrap < 20 {
		wrap = 20
	}
	if wrap > 64 {
		wrap = 64
	}

	border := brandRed
	if c := paletteColor(outline); c != "" {
		border = c
	}
	textStyle := lipgloss.NewStyle().Bold(true).Foreground(brightColor)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1)
	if bgCol := paletteColor(bg); bgCol != "" {
		textStyle = textStyle.Foreground(mindInk).Background(bgCol)
		box = box.Background(bgCol)
	}
	if ansi.StringWidth(full) > wrap {
		textStyle = textStyle.Width(wrap) // wrap only when the text is too long
	}
	card := box.Render(textStyle.Render(full))
	return strings.Split(card, "\n")
}

// overlayAt composites the ov lines onto bg starting at (top, left), preserving
// the background to the left and right of the overlay. ANSI-aware so styled map
// cells survive the cut.
func overlayAt(bg, ov []string, top, left int) []string {
	out := append([]string(nil), bg...)
	for len(out) < top+len(ov) {
		out = append(out, "")
	}
	for i, ovl := range ov {
		y := top + i
		bgLine := out[y]
		ovW := ansi.StringWidth(ovl)
		leftSeg := ansi.Truncate(bgLine, left, "")
		if pad := left - ansi.StringWidth(leftSeg); pad > 0 {
			leftSeg += strings.Repeat(" ", pad)
		}
		rightSeg := ansi.TruncateLeft(bgLine, left+ovW, "")
		out[y] = leftSeg + ovl + rightSeg
	}
	return out
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// mindAction is one runnable entry in the mind-map quick-action palette. msg is
// dispatched into updateMindMap so the palette reuses the real key handlers.
type mindAction struct {
	key   string // display label for the key
	label string
	msg   tea.KeyMsg
}

func mindRuneMsg(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

var mindPaletteActions = []mindAction{
	{"tab", "Add child node", tea.KeyMsg{Type: tea.KeyTab}},
	{"enter", "Add sibling node", tea.KeyMsg{Type: tea.KeyEnter}},
	{"r", "Jump to the root node", mindRuneMsg('r')},
	{"R", "Rename the root (the idea)", mindRuneMsg('R')},
	{"e", "Edit node text", mindRuneMsg('e')},
	{"t", "Mark / unmark as task", mindRuneMsg('t')},
	{"T", "Convert tasks → a project", mindRuneMsg('T')},
	{"U", "Unbind project & unlink tasks", mindRuneMsg('U')},
	{"x", "Complete / reopen task node", mindRuneMsg('x')},
	{"d", "Delete node and subtree", mindRuneMsg('d')},
	{"s", "Sync with Todoist", mindRuneMsg('s')},
	{"c", "Outline colour — this node", mindRuneMsg('c')},
	{"C", "Outline colour — node + children", mindRuneMsg('C')},
	{"v", "Background fill — this node", mindRuneMsg('v')},
	{"V", "Background fill — node + children", mindRuneMsg('V')},
	{"space", "Collapse / expand branch", mindRuneMsg(' ')},
	{"H", "Mind-map help", mindRuneMsg('H')},
	{"esc", "Save & back to ideas", tea.KeyMsg{Type: tea.KeyEsc}},
}

// mindPalFiltered returns the palette actions matching the type-to-filter query.
func (m model) mindPalFiltered() []mindAction {
	q := strings.ToLower(strings.TrimSpace(m.palQuery))
	if q == "" {
		return mindPaletteActions
	}
	var out []mindAction
	for _, a := range mindPaletteActions {
		if strings.Contains(strings.ToLower(a.label), q) || strings.ToLower(a.key) == q {
			out = append(out, a)
		}
	}
	return out
}

func (m model) updateMindPalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matches := m.mindPalFiltered()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeMindMap
		return m, nil
	case "enter":
		m.mode = modeMindMap
		if len(matches) == 0 {
			return m, nil
		}
		if m.palCursor >= len(matches) {
			m.palCursor = len(matches) - 1
		}
		return m.updateMindMap(matches[m.palCursor].msg)
	case "up":
		if m.palCursor > 0 {
			m.palCursor--
		}
		return m, nil
	case "down":
		if m.palCursor < len(matches)-1 {
			m.palCursor++
		}
		return m, nil
	case "backspace":
		if len(m.palQuery) > 0 {
			m.palQuery = m.palQuery[:len(m.palQuery)-1]
			m.palCursor = 0
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.palQuery += string(msg.Runes)
			m.palCursor = 0
		}
		return m, nil
	}
}

// mindPaletteView renders the mind-map quick-action palette (` to open).
func (m model) mindPaletteView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	keyStyle := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	matches := m.mindPalFiltered()

	lines := []string{"", "  " + accent.Render("🗺  Mind-map action") + dim.Render("   search: ") + m.palQuery + "▏", ""}

	const win = 10
	start := 0
	if m.palCursor >= win {
		start = m.palCursor - win + 1
	}
	end := start + win
	if end > len(matches) {
		end = len(matches)
	}
	if len(matches) == 0 {
		lines = append(lines, "  "+dim.Render("no matching action — try 'task', 'colour', 'child'…"))
	}
	for i := start; i < end; i++ {
		a := matches[i]
		cur := "   "
		key := keyStyle.Render(fmt.Sprintf("%5s", a.key))
		label := dim.Render(a.label)
		if i == m.palCursor {
			cur = accent.Render(" ▸ ")
			label = lipgloss.NewStyle().Foreground(brightColor).Bold(true).Render(a.label)
		}
		lines = append(lines, cur+key+"   "+label)
	}
	lines = append(lines, "", helpStyle.Render("  type to filter · ↑/↓ move · enter run · esc cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
}

// mindConfirmDeleteView asks to confirm deleting a node (and its subtree).
func (m model) mindConfirmDeleteView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	body := lipgloss.NewStyle().Foreground(textColor)

	name := "(node)"
	kids := 0
	if m.mindDelTarget != nil {
		name = strings.ReplaceAll(strings.TrimSpace(m.mindDelTarget.Text), "\n", " ")
		if name == "" {
			name = "(empty)"
		}
		kids = m.mindDelTarget.countNodes()
	}
	what := "Delete this node?"
	if kids > 0 {
		what = fmt.Sprintf("Delete this node and its %d child node(s)?", kids)
	}
	rows := []string{
		accent.Render("🗺  " + what),
		"",
		body.Render("  " + mmTruncate(name, 60)),
		"",
		dim.Render("Any linked Todoist task is left in place."),
		"",
		accent.Render("y") + dim.Render(" delete    ") + accent.Render("n") + dim.Render(" cancel"),
	}
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(brandRed).
		Padding(1, 3).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	bodyH := m.height - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, header,
		lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card))
}

// mindConfirmUnbindView asks to confirm unbinding the idea's project.
func (m model) mindConfirmUnbindView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	proj := ""
	if m.mindIdea >= 0 && m.mindIdea < len(m.ideas) {
		proj = m.ideas[m.mindIdea].ProjectName
	}
	rows := []string{
		accent.Render("🗺  Unbind from " + proj + "?"),
		"",
		dim.Render("This unlinks every generated task in this map."),
		dim.Render("The tasks already in Todoist are left in place."),
		"",
		accent.Render("y") + dim.Render(" unbind    ") + accent.Render("n") + dim.Render(" cancel"),
	}
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(brandRed).
		Padding(1, 3).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	bodyH := m.height - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, header,
		lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card))
}

// countNodes on a MindNode counts itself's descendants (helper for the dialog).
func (n *MindNode) countNodes() int {
	c := 0
	for _, k := range n.Children {
		c += 1 + k.countNodes()
	}
	return c
}

// mindHelpView is the dedicated keyboard reference for mind-map mode (H / ?).
func (m model) mindHelpView(header string) string {
	yellow := lipgloss.NewStyle().Foreground(dueColor).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	head := lipgloss.NewStyle().Foreground(brightColor).Bold(true)

	row := func(k, desc string) string {
		return "  " + keyStyle.Width(14).Render(k) + lipgloss.NewStyle().Foreground(textColor).Render(desc)
	}

	// A small swatch strip so the 10 colours are visible at a glance.
	var swatches []string
	for _, c := range mindPalette {
		swatches = append(swatches, lipgloss.NewStyle().Background(c).Render("  "))
	}

	rows := []string{
		yellow.Render("🗺  Mind-map keys"),
		"",
		head.Render("  Navigate"),
		row("↑ / ↓ · j/k", "Previous / next sibling (or nearest node)"),
		row("← / h", "Go to parent (one column left)"),
		row("→ / l", "Descend to the first child"),
		row("r", "Jump to the root node (left-most)"),
		row("R", "Rename the root node (the idea itself)"),
		row("z", "Zoom — overlay the selected node's full text on the map"),
		row("", "the map stays navigable underneath; z / esc closes it"),
		"",
		head.Render("  Edit the tree"),
		row("Tab", "Add a child of the selected node"),
		row("Enter", "Add a sibling after the selected node"),
		row("e / i", "Edit the selected node's text"),
		row("Space", "Collapse / expand a branch"),
		row("d / del", "Delete the node and its subtree (asks y/n)"),
		"",
		head.Render("  Tasks"),
		row("t", "Mark / unmark as a task ([ ] checkbox)"),
		row("T", "Convert marked tasks → a project (creates it if new)"),
		row("", "an idea binds to ONE project; T tops up new tasks"),
		row("U", "Unbind the project & unlink the generated tasks"),
		row("x", "Complete / reopen a task node ([x]); ignored on non-tasks"),
		row("s", "Sync — push tasks & pull completions from Todoist"),
		row("", "completing in Todoist also ticks it here on sync"),
		"",
		head.Render("  Colour (10 colours, cycles)"),
		row("c", "Cycle this node's outline colour"),
		row("C", "Outline colour for node + all children"),
		row("v", "Cycle this node's background fill"),
		row("V", "Background fill for node + all children"),
		"  " + dim.Render("palette: ") + strings.Join(swatches, ""),
		"",
		head.Render("  Leave"),
		row("esc / q / b", "Save and return to the idea list (b = back)"),
		row("H / ?", "This help (press any key to close)"),
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dueColor).
		Padding(1, 3).
		Render(body)

	bodyH := m.height - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, header,
		lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card))
}
