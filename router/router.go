package router

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/vela-games/lfsproxy/config"
	"github.com/vela-games/lfsproxy/exporter"
	"github.com/vela-games/lfsproxy/handlers"
)

type Router struct {
	engine *gin.Engine
}

func NewRouter() Router {
	if viper.GetBool("debug_mode") {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	gin := gin.Default()
	gin.Use(cors.Default())

	return Router{
		engine: gin,
	}
}

func (r Router) InitRoutes(ctx context.Context, cfg *config.Config) error {
	healthHandler := handlers.HealthHandler{}

	lfsHandler, err := handlers.NewLFSHandler(ctx, cfg)
	if err != nil {
		return err
	}

	r.engine.Use(gzip.Gzip(gzip.DefaultCompression))
	r.engine.GET("/health", healthHandler.Get)
	r.engine.POST("/objects/batch", lfsHandler.PostBatch)

	if cfg.EnablePrometheusExporter {
		r.engine.GET("/metrics", exporter.PrometheusHandler())
	}

	return nil
}

func (r Router) Run(ctx context.Context, portBinding string) error {
	srv := &http.Server{
		Addr:              portBinding,
		Handler:           r.engine,
		IdleTimeout:       5 * time.Minute,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go r.listen(srv)
	<-ctx.Done()
	log.Println("shutting down server")

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(timeoutCtx); err != nil {
		return err
	}

	return nil
}

func (r Router) listen(srv *http.Server) {
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("error trying to listen: %s\n", err)
	}
}
