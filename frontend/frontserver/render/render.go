package render

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/diamondburned/smolboard/client"
	"github.com/diamondburned/smolboard/smolboard"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/phogolabs/parcello"
)

// Renderer represents a renderable page.
type Renderer = func(r *Request) (Render, error)

// ErrorRenderer represents a renderable page for errors.
type ErrorRenderer = func(r *Request, err error) (Render, error)

type Render struct {
	Title       string // og:title, <title>
	Description string // og:description
	ImageURL    string // og:image

	Body template.HTML
}

// Empty is a blank page.
var Empty = Render{}

type Config struct {
	SiteName string `toml:"siteName"`
}

func NewConfig() Config {
	return Config{
		SiteName: "smolboard",
	}
}

func (c *Config) Validate() error {
	return nil
}

type renderCtx struct {
	Theme     Theme
	Render    Render
	Config    Config
	BodyClass string
}

func (r renderCtx) FormatTitle() string {
	if r.Render.Title == "" {
		return r.Config.SiteName
	}
	return fmt.Sprintf("%s - %s", r.Render.Title, r.Config.SiteName)
}

type Request struct {
	CommonCtx

	Writer http.ResponseWriter
	pusher http.Pusher
}

// TokenCookie returns the cookie that contains the token if any or nil if
// none. This function searches the cookie from the smolboard client.
func (r *Request) TokenCookie() *http.Cookie {
	for _, cookie := range r.Session.Client.Cookies() {
		if cookie.Name == "token" {
			return cookie
		}
	}
	return nil
}

// Cookie returns the cookie from the Request. These cookies won't have
// anything else filled other than the value field.
func (r *Request) Cookie(name string) *http.Cookie {
	for _, cookie := range r.Request.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func (r *Request) CookieValue(name string) string {
	if c := r.Cookie(name); c != nil {
		return c.Value
	}
	return ""
}

var (
	maxExpire  = time.Unix(math.MaxInt32, 0)
	zeroExpire = time.Unix(0, 0)
)

// SetWeakCookie sets an insecure cookie for user settings.
func SetWeakCookie(w http.ResponseWriter, k, v string) {
	var expires = maxExpire
	if v == "" {
		expires = zeroExpire
	}

	http.SetCookie(w, &http.Cookie{
		Name:     k,
		Value:    v,
		Path:     "/",
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetWeakCookie calls SetWeakCookie with the writer in Request.
func (r *Request) SetWeakCookie(k, v string) {
	SetWeakCookie(r.Writer, k, v)
}

// FlushCookies dumps all the session state's cookies to the response writer.
func (r *Request) FlushCookies() {
	// // We should only flush cookies newer than current time. This is because
	// // cookies that don't have expiry times are from the browser, so we don't
	// // need to echo the same cookie back.
	// var now = time.Now()

	for _, cookie := range r.Session.Client.Cookies() {
		http.SetCookie(r.Writer, cookie)

		// Is this a token cookie? Is it invalidated? If yes, then we should
		// invalidate the username cookie too.
		if cookie.Name == "token" && cookie.Value == "" {
			http.SetCookie(r.Writer, &http.Cookie{
				Name:    "username",
				Value:   "",
				Path:    "/",
				Expires: zeroExpire, // set to the past
			})
		}
	}
}

func (r *Request) Push(url string) {
	if r.pusher == nil {
		ps, ok := r.Writer.(http.Pusher)
		if !ok {
			return
		}
		r.pusher = ps
	}

	r.pusher.Push(url, nil)
}

func (r *Request) Param(name string) string {
	return chi.URLParam(r.Request, name)
}

// IDParam returns the ID parameter from chi.
func (r *Request) IDParam() (int64, error) {
	return strconv.ParseInt(r.Param("id"), 10, 64)
}

func (r *Request) Redirect(path string, code int) {
	// Flush the cookies before writing the header.
	r.FlushCookies()
	http.Redirect(r.Writer, r.Request, path, code)
}

func (r *Request) Me() (smolboard.UserPart, error) {
	u, err := r.Session.Me()
	if err != nil {
		return u, err
	}

	r.Username = u.Username
	return u, nil
}

type CommonCtx struct {
	*http.Request

	Config   Config
	Username string
	Session  *client.Session
}

func pushAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pusher, ok := w.(http.Pusher); ok {
			// Push over the CSS to save a trip.
			pusher.Push("/static/components.css", nil)

			// Push over all the theme CSS.
			for i := Theme(0); i < themeLen; i++ {
				pusher.Push(i.URL(), nil)
			}
		}
	})
}

type Mux struct {
	*chi.Mux
	client func(r *http.Request) (*client.Client, error)
	cfg    Config
	errR   ErrorRenderer
}

func newMux() *chi.Mux {
	ensureInit()

	r := chi.NewMux()
	r.Use(ThemeM)
	r.Use(middleware.StripSlashes)

	d, err := parcello.Manager.Dir("static/")
	if err != nil {
		log.Fatalln("Static not found:", err)
	}

	r.With(middleware.NoCache).Post("/theme", handleSetTheme)
	r.Route("/static", func(r chi.Router) {
		r.Get("/components.css", componentsCSSHandler)
		r.Mount("/", http.StripPrefix("/static", http.FileServer(d)))
	})

	return r
}

// NewMux creates a new frontend muxer with the given backendSocket as the Unix
// socket path to call the backend.
func NewMux(backendSocket string, cfg Config) *Mux {
	return &Mux{
		Mux: newMux(),
		cfg: cfg,
		client: func(r *http.Request) (*client.Client, error) {
			return client.NewSocketClientFromRequest(backendSocket, r)
		},
	}
}

// NewHTTPMux creates a new frontend muxer with the given backendHTTP endpoint.
// This endpoint will be used to make HTTP requests to the server.
func NewHTTPMux(backendHTTP string, cfg Config) *Mux {
	return &Mux{
		Mux: newMux(),
		cfg: cfg,
		client: func(r *http.Request) (*client.Client, error) {
			return client.NewHTTPClientFromRequest(backendHTTP, r)
		},
	}
}

func (m *Mux) SetErrorRenderer(r ErrorRenderer) {
	m.errR = r
}

// NewRequest makes a new internal request struct. The returned Request pointer
// is never nil.
func (m *Mux) NewRequest(w http.ResponseWriter, r *http.Request) *Request {
	c, err := m.client(r)
	if err != nil {
		// Host is a constant, so we can panic here.
		log.Panicln("Error making client:", err)
	}

	s := client.NewSessionWithClient(c)

	request := &Request{
		CommonCtx: CommonCtx{
			Config:  m.cfg,
			Request: r,
			Session: s,
		},
		Writer: w,
	}

	// Try and grab the username from the cookies. Only use this username value
	// for visual purposes, such as displaying.
	if c, err := r.Cookie("username"); err == nil {
		request.CommonCtx.Username = c.Value

	} else {
		// Update the username cookie if there's a token cookie.
		if c, err := r.Cookie("token"); err == nil && c.Value != "" {
			u, err := s.Me()
			if err == nil {
				request.CommonCtx.Username = u.Username

				http.SetCookie(w, &http.Cookie{
					Name:  "username",
					Value: u.Username,
					Path:  "/",
					// We're not setting an Expiry here so the cookie will
					// expire when the browser exits.
				})
			}
		}
	}

	return request
}

// M is the middleware wrapper.
func (m *Mux) M(render Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Write the proper headers.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var page Render

		// Make a new request. If that works, then we render the page.
		request := m.NewRequest(w, r)
		page, err := render(request)

		// If either of the above failed, then render it as an error page.
		if err != nil {
			page = m.renderError(err, request)
		} else {
			// Flush the cookies before writing the body if there is no error.
			request.FlushCookies()
		}

		// Don't render anything if an empty page is returned and there is no
		// error.
		if page == Empty {
			return
		}

		var renderCtx = renderCtx{
			Theme:  GetTheme(r.Context()),
			Render: page,
			Config: m.cfg,
		}

		if err := index.Execute(w, renderCtx); err != nil {
			return
		}
	}
}

func (m *Mux) renderError(err error, r *Request) Render {
	// Copy the status code if available. Else, fallback to 500.
	r.Writer.WriteHeader(client.ErrGetStatusCode(err, 500))

	// If there is no error renderer, then we just write the error down
	// in plain text.
	if m.errR == nil {
		fmt.Fprintf(r.Writer, "Error: %v", err)
		return Empty
	}

	// Render the error page.
	page, err := m.errR(r, err)
	if err != nil {
		// This shouldn't error out, so we should log it.
		log.Println("Error rendering error page:", err)
		return Empty
	}

	return page
}

func (m *Mux) Get(route string, r Renderer) {
	m.Mux.With(middleware.NoCache).Get(route, m.M(r))
}

func (m *Mux) Post(route string, r Renderer) {
	m.Mux.With(middleware.NoCache).Post(route, m.M(r))
}

func (m *Mux) Delete(route string, r Renderer) {
	m.Mux.With(middleware.NoCache).Delete(route, m.M(r))
}

// Muxer implements the interface that's passable to pages' mount functions.
type Muxer interface {
	M(Renderer) http.HandlerFunc
}

func (m *Mux) Mount(route string, mounter func(Muxer) http.Handler) {
	m.Mux.With(middleware.NoCache).Mount(route, mounter(m))
}
