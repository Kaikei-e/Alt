package server

import (
	"context"
	"github.com/meilisearch/meilisearch-go"
	"net/http"
	"search-indexer/config"
	"search-indexer/logger"
	"search-indexer/rest"
)

type Server struct {
	config *config.Config
	server *http.Server
	index  meilisearch.IndexManager
}

func New(cfg *config.Config, idx meilisearch.IndexManager) *Server {
	mux := http.NewServeMux()

	s := &Server{
		config: cfg,
		index:  idx,
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           mux,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		},
	}

	s.setupRoutes(mux)
	return s
}

func (s *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		rest.SearchArticles(w, r, s.index)
	})
}

func (s *Server) Start() error {
	logger.Logger.Info("starting HTTP server", "addr", s.config.HTTP.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	logger.Logger.Info("shutting down HTTP server")
	return s.server.Shutdown(ctx)
}
