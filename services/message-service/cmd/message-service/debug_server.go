package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("message-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("message-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func messageDebugAddr() string {
	return envString("NEXUSIM_MESSAGE_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func messageDebugAddrFromEnv() (string, error) {
	addr := messageDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateMessageDebugListenerConfig(addr, allowPublic)
}

func validateMessageDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("message-service debug listener address is non-private; set NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC=true to allow")
}
