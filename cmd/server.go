package main

import (
	"fmt"
	"log"

	"github.com/vela-games/lfsproxy/config"
	"github.com/vela-games/lfsproxy/router"

	"context"
	"os"
	"os/signal"
	"syscall"
)

const PORT = 8080

// NewSigKillContext returns a Context that cancels when os.Interrupt or os.Kill is received
func NewSigKillContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
	}()

	return ctx
}

func main() {
	ctx := NewSigKillContext()
	cfg, err := config.GetConfig()
	if err != nil {
		log.Panicf("error getting configuration: %v", err)
	}

	router := router.NewRouter()
	err = router.InitRoutes(ctx, cfg)
	if err != nil {
		panic(err)
	}

	err = router.Run(ctx, fmt.Sprintf(":%v", PORT))
	if err != nil {
		panic(err)
	}

}
