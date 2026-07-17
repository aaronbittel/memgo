package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	port := flag.Int("port", DefaultPort, "specify port")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := newServer(logger)

	addr := fmt.Sprintf(":%d", *port)

	if err := server.ListenAndServe(addr); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}

	logger.Info("server closed")
}
