package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bionicpro-auth/internal/api"
	"bionicpro-auth/internal/config"
	"bionicpro-auth/internal/keycloak"
	"bionicpro-auth/internal/profile"
	"bionicpro-auth/internal/session"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := session.NewStore(cfg.EncryptionKey, cfg.SessionTTL)
	if err != nil {
		log.Fatalf("create session store: %v", err)
	}

	profileStore, err := profile.Open(cfg.ProfileDBPath)
	if err != nil {
		log.Fatalf("open profile store: %v", err)
	}
	defer profileStore.Close()

	kcClient := keycloak.NewClient(cfg)
	handler := api.NewServer(cfg, store, kcClient, profileStore)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("bionicpro-auth listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown failed: %v", err)
	}
}
