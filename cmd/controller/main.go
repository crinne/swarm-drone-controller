package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/crinne/swarm-drone-controller/internal/httpapi"
	"github.com/crinne/swarm-drone-controller/internal/kube"
	"github.com/crinne/swarm-drone-controller/internal/spawn"
)

const maxDrones = 20

func main() {
	if err := run(); err != nil {
		log.Printf("event=controller_error msg=%q", err.Error())
		os.Exit(1)
	}
}

func run() error {
	namespace, err := requiredEnv("NAMESPACE")
	if err != nil {
		return err
	}
	droneImage, err := requiredEnv("DRONE_IMAGE")
	if err != nil {
		return err
	}
	spawnPassword, err := requiredEnv("SPAWN_PASSWORD")
	if err != nil {
		return err
	}
	allowedOrigin, err := requiredEnv("ALLOWED_ORIGIN")
	if err != nil {
		return err
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("load in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	store := kube.NewStore(client, namespace, droneImage)
	service := spawn.NewService(store, spawnPassword, maxDrones)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           httpapi.NewHandler(service, allowedOrigin),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("event=controller_start addr=%s namespace=%s", server.Addr, namespace)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		log.Print("event=controller_stop")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	}
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("missing required env %s", name)
	}
	return value, nil
}
