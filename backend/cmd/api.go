package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/events"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	"github.com/IkBenJur/italy-trip/internal/photos"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/IkBenJur/italy-trip/internal/users"
	"github.com/gin-gonic/gin"
)

type Application struct {
	Port           string
	AllowedOrigins []string
	Queries        repo.Querier
	Issuer         *auth.TokenIssuer
	Storage        storage.Storage
	MaxUploadBytes int64
}

func (app *Application) Mount() http.Handler {
	router := gin.Default()

	router.Use(middleware.CORS(app.AllowedOrigins))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	})

	userHandler := users.NewHandler(app.Queries, app.Issuer)
	eventHandler := events.NewHandler(app.Queries)

	// There is no registration route. With one shared account and an album that
	// unlocks for any authenticated user, an open sign-up would let a stranger
	// register today and read the whole album later.
	router.POST("/auth/login", userHandler.Login)

	photoHandler := photos.NewHandler(app.Queries, app.Storage, app.MaxUploadBytes)

	authorized := router.Group("/", middleware.RequireAuth(app.Issuer, app.Queries))
	authorized.GET("users/me", userHandler.Me)
	authorized.GET("events/current", eventHandler.Current)
	authorized.POST("events", eventHandler.Create)
	authorized.POST("events/current/photos", photoHandler.Upload)
	authorized.GET("events/current/photos", photoHandler.List)
	authorized.GET("photos/:id/original", photoHandler.Original)

	return router
}

func (app *Application) Run(ctx context.Context, router http.Handler) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", app.Port),
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	return srv.ListenAndServe()
}
