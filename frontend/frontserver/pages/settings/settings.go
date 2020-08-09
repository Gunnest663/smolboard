package settings

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/diamondburned/smolboard/client"
	"github.com/diamondburned/smolboard/frontend/frontserver/components/footer"
	"github.com/diamondburned/smolboard/frontend/frontserver/components/nav"
	"github.com/diamondburned/smolboard/frontend/frontserver/pages/settings/tokenlist"
	"github.com/diamondburned/smolboard/frontend/frontserver/pages/settings/userlist"
	"github.com/diamondburned/smolboard/frontend/frontserver/render"
	"github.com/diamondburned/smolboard/smolboard"
	"github.com/go-chi/chi"
	"github.com/markbates/pkger"
	"github.com/pkg/errors"

	ua "github.com/mileusna/useragent"
)

func init() {
	render.RegisterCSSFile(
		pkger.Include("/frontend/frontserver/pages/settings/settings.css"),
	)
}

var tmpl = render.BuildPage("cpanel", render.Page{
	Template: pkger.Include("/frontend/frontserver/pages/settings/settings.html"),
	Components: map[string]render.Component{
		"nav":    nav.Component,
		"footer": footer.Component,
	},
	Functions: template.FuncMap{
		"userAgent": func(s string) ua.UserAgent {
			return ua.Parse(s)
		},
		"unixNano": func(i int64) time.Time {
			return time.Unix(0, i)
		},
		"uaEmoji": func(u ua.UserAgent) string {
			switch {
			case u.Mobile, u.Tablet:
				return "📱"
			case u.Desktop:
				return "🖥️"
			case u.Bot:
				return "🤖"
			default:
				return "❓"
			}
		},
	},
})

// Normal user stuff.
type renderCtx struct {
	render.CommonCtx
	Current  smolboard.UserPart
	Sessions []smolboard.Session
}

func (r renderCtx) IsAdmin() bool {
	return r.Current.Permission >= smolboard.PermissionAdministrator
}

func Mount(muxer render.Muxer) http.Handler {
	mux := chi.NewMux()
	mux.Get("/", muxer.M(userPanel))
	mux.Post("/sessions/{sessionID}/delete", muxer.M(nil))

	mux.Mount("/tokens", tokenlist.Mount(muxer))

	mux.Route("/users", func(mux chi.Router) {
		mux.Route("/@me", func(mux chi.Router) {
			mux.Post("/delete", muxer.M(deleteUser))
			mux.Post("/change-password", muxer.M(changePassword))
		})

		mux.Mount("/", userlist.Mount(muxer))
	})

	return mux
}

func userPanel(r *render.Request) (render.Render, error) {
	u, err := r.Session.Me()
	if err != nil {
		return render.Empty, errors.Wrap(err, "Failed to get current user")
	}

	s, err := r.Session.GetSessions()
	if err != nil {
		return render.Empty, errors.Wrap(err, "Failed to get sessions")
	}

	return render.Render{
		Title: "Settings",
		Body: tmpl.Render(renderCtx{
			CommonCtx: r.CommonCtx,
			Current:   u,
			Sessions:  s,
		}),
	}, nil
}

func deleteUser(r *render.Request) (render.Render, error) {
	if err := r.Session.DeleteMe(); err != nil {
		return render.Empty, nil
	}

	r.Redirect("/", http.StatusSeeOther)
	return render.Empty, nil
}

func changePassword(r *render.Request) (render.Render, error) {
	_, err := r.Session.EditMe(client.UserEditParams{
		Password: r.FormValue("password"),
	})
	if err != nil {
		return render.Empty, err
	}

	r.Redirect(r.Referer(), http.StatusSeeOther)
	return render.Empty, nil
}

func deleteSession(r *render.Request) (render.Render, error) {
	i, err := strconv.ParseInt(chi.URLParam(r.Request, "sessionID"), 10, 64)
	if err != nil {
		return render.Empty, errors.Wrap(err, "Failed to parse session ID")
	}

	if err := r.Session.DeleteSession(i); err != nil {
		return render.Empty, err
	}

	r.Redirect(r.Referer(), http.StatusSeeOther)
	return render.Empty, nil
}
