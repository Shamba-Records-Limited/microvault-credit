package views

import (
	"strings"
	"sync"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/static"
	"github.com/a-h/templ"
)

// logoSVG is the mark with its opening tag pre-marked decorative and its
// intrinsic 256px dimensions cut to icon size. Presentation attributes lose to
// CSS, so a size class still wins; the small default only matters if that class
// is missing, where it keeps a stale stylesheet from blowing up the layout.
var logoSVG = sync.OnceValue(func() string {
	raw, err := static.FS.ReadFile("img/microvault-logo.svg")
	if err != nil {
		return ""
	}
	svg := string(raw)
	svg = strings.Replace(svg, `width="256" height="256"`, `width="24" height="24"`, 1)
	return strings.Replace(svg, "<svg ", `<svg aria-hidden="true" focusable="false" `, 1)
})

// Logo renders the Microvault mark at the size given by class.
func Logo(class string) templ.Component {
	svg := logoSVG()
	if svg == "" {
		return templ.NopComponent
	}
	return templ.Raw(strings.Replace(svg, "<svg ", `<svg class="`+templ.EscapeString(class)+`" `, 1))
}
