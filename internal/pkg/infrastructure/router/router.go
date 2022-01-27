package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog"
	"github.com/riandyrn/otelchi"
	"github.com/rs/cors"
)

func New(serviceName string) *chi.Mux {
	r := chi.NewRouter()

	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	r.Use(httplog.RequestLogger(
		httplog.NewLogger(serviceName, httplog.Options{
			JSON: true,
		}),
	))

	r.Use(otelchi.Middleware(serviceName, otelchi.WithChiRoutes(r)))

	return r
}
