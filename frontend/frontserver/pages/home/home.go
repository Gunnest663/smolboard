package home

import (
	"github.com/diamondburned/smolboard/frontend/frontserver/render"

	// Components
	"github.com/diamondburned/smolboard/frontend/frontserver/components/footer"
	"github.com/diamondburned/smolboard/frontend/frontserver/components/search"
)

func init() {
	render.RegisterCSSFile("pages/home/home.css")
}

var tmpl = render.BuildPage("home", render.Page{
	Template: "pages/home/home.html",
	Components: map[string]render.Component{
		"search": search.Component,
		"footer": footer.Component,
	},
})

type renderCtx struct {
	render.CommonCtx
}

func Render(r *render.Request) (render.Render, error) {
	return render.Render{
		Body: tmpl.Render(renderCtx{
			CommonCtx: r.CommonCtx,
		}),
	}, nil
}
