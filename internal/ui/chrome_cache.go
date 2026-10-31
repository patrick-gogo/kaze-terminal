package ui

import (
	"slices"

	"gioui.org/io/system"
	"gioui.org/text"
	"gioui.org/unit"
)

// Geometry caches retain only the last inputs and rectangles, never Gio ops.
// Labels still draw into each frame's fresh Ops, with that frame's colors.
type statusLayoutCache struct {
	valid  bool
	model  statusModel
	width  int
	dims   statusDims
	metric unit.Metric
	font   *text.Shaper
	value  statusLayout
}

func sameStatus(a, b statusModel) bool {
	return a.Folder == b.Folder && a.Branch == b.Branch && a.Add == b.Add && a.Del == b.Del &&
		a.Update == b.Update && a.Agents == b.Agents && a.Waiting == b.Waiting && a.MemMB == b.MemMB &&
		a.Tabs == b.Tabs && a.Notice == b.Notice && slices.Equal(a.Servers, b.Servers) && slices.Equal(a.Usage, b.Usage)
}

func (c *statusLayoutCache) layout(m statusModel, w int, d statusDims, metric unit.Metric, font *text.Shaper, measure func(string, bool) int) statusLayout {
	if c.valid && c.width == w && c.dims == d && c.metric == metric && c.font == font && sameStatus(c.model, m) {
		return c.value
	}
	servers := append(c.model.Servers[:0], m.Servers...)
	usage := append(c.model.Usage[:0], m.Usage...)
	c.model = m
	c.model.Servers = servers
	c.model.Usage = usage
	c.width, c.dims, c.metric, c.font = w, d, metric, font
	c.value = layoutStatusBar(m, w, measure, d)
	c.valid = true
	return c.value
}

type titleLayoutCache struct {
	valid  bool
	in     titleIn
	locale system.Locale
	metric unit.Metric
	font   *text.Shaper
	value  titleLayout
}

func (c *titleLayoutCache) layout(in titleIn, metric unit.Metric, font *text.Shaper) titleLayout {
	old := c.in
	if c.valid && old.Size == in.Size && old.Mac == in.Mac && old.LightsW == in.LightsW && old.SideW == in.SideW &&
		old.Crumb == in.Crumb && old.Key == in.Key && old.Dims == in.Dims && c.metric == metric && c.font == font {
		return c.value
	}
	c.value = layoutTitle(in)
	c.in = in
	c.in.Measure = nil
	c.metric, c.font, c.valid = metric, font, true
	return c.value
}
