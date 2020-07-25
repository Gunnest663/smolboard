package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/c2h5oh/datasize"
	"github.com/diamondburned/smolboard/smolboard/db"
	"github.com/diamondburned/smolboard/smolboard/http/internal/limit"
	"github.com/diamondburned/smolboard/smolboard/http/internal/middleware"
	"github.com/diamondburned/smolboard/smolboard/http/internal/tx"
	"github.com/diamondburned/smolboard/smolboard/http/post"
	"github.com/diamondburned/smolboard/smolboard/http/token"
	"github.com/diamondburned/smolboard/smolboard/http/upload"
	"github.com/diamondburned/smolboard/smolboard/http/upload/imgsrv"
	"github.com/diamondburned/smolboard/smolboard/http/user"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
)

type HTTPConfig struct {
	Address     string            `toml:"address"`
	MaxBodySize datasize.ByteSize `toml:"maxBodySize"`

	// inherit upload's config
	upload.UploadConfig
}

func NewConfig() HTTPConfig {
	return HTTPConfig{
		Address:      "",
		MaxBodySize:  1 * datasize.GB,
		UploadConfig: upload.NewConfig(),
	}
}

func (c *HTTPConfig) Validate() error {
	if c.Address == "" {
		return errors.New("Missing `address'")
	}

	return c.UploadConfig.Validate()
}

func GetTypes(cfg HTTPConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(cfg.AllowedTypes); err != nil {
			log.Println("Encode failed:", err)
		}
	}
}

type Routes struct {
	http.Handler
	mw  tx.Middleware
	cfg HTTPConfig
}

func New(db *db.Database, cfg HTTPConfig) (*Routes, error) {
	mux := chi.NewMux()
	rts := &Routes{
		Handler: mux,
		mw:      tx.NewMiddleware(db, cfg.UploadConfig),
		cfg:     cfg,
	}

	// Alias the middleware function.
	m := rts.mw.M

	mux.Use(
		middleware.RealIP,
		middleware.Recoverer,
		middleware.Compress(5),
		middleware.LimitBody(cfg.MaxBodySize),
	)

	mux.Group(func(r chi.Router) {
		mux.Use(limit.RateLimit(2))
		mux.Post("/signin", m(user.Signin))
		mux.Post("/signup", m(user.Signup))
		mux.Post("/signout", m(user.Signout))
	})

	mux.Group(func(r chi.Router) {
		mux.Use(limit.RateLimit(64))
		mux.Get("/filetypes", GetTypes(cfg))
	})

	mux.Mount("/tokens", token.Mount(m))
	mux.Mount("/image", imgsrv.Mount(m))
	mux.Mount("/posts", post.Mount(m))
	mux.Mount("/users", user.Mount(m))

	return rts, nil
}

func (r *Routes) ListenAndServe() error {
	return http.ListenAndServe(r.cfg.Address, r)
}
