// +build !solution

package artifact

import (
	"fmt"
	"gitlab.com/slon/shad-go/distbuild/pkg/build"
	"gitlab.com/slon/shad-go/distbuild/pkg/tarstream"
	"net/http"

	"go.uber.org/zap"
)

type Handler struct {
	logger *zap.Logger
	cache  *Cache
}

func NewHandler(l *zap.Logger, c *Cache) *Handler {
	return &Handler{
		logger: l.Named("artifact handler"),
		cache:  c,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	if r.Method != http.MethodGet {
		h.logger.Error(fmt.Sprintf("incorrect method"))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var artifactID build.ID
	err := artifactID.UnmarshalText([]byte(r.URL.Query().Get("id")))
	if err != nil {
		h.logger.Error(fmt.Sprintf("decoding url: %v", err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	path, unlock, err := h.cache.Get(artifactID)
	if err != nil {
		h.logger.Error(fmt.Sprintf("cache get: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	defer unlock()
	err = tarstream.Send(path, w)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/artifact", h)
}
