// +build !solution

package filecache

import (
	"errors"
	"fmt"
	"gitlab.com/slon/shad-go/distbuild/pkg/build"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"io"
	"net/http"
	"net/url"
	"os"
)

type Handler struct {
	logger *zap.Logger
	cache  *Cache
	group  singleflight.Group
}

func NewHandler(l *zap.Logger, cache *Cache) *Handler {
	return &Handler{
		logger: l.Named("filecache handler"),
		cache:  cache,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getHandler(w, r)
		return
	case http.MethodPut:
		h.putHandler(w, r)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func extractID(uri *url.URL) (id build.ID, err error) {
	err = id.UnmarshalText([]byte(uri.Query().Get("id")))
	return
}

func (h *Handler) getHandler(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	id, err := extractID(r.URL)
	if err != nil {
		h.logger.Error(fmt.Sprintf("decoding url: %v", err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	path, unlock, err := h.cache.Get(id)
	if err != nil {
		h.logger.Error(fmt.Sprintf("cache get: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	defer unlock()
	f, err := os.Open(path)
	if err != nil {
		h.logger.Error(fmt.Sprintf("file open: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func (h *Handler) putHandler(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	id, err := extractID(r.URL)
	if err != nil {
		h.logger.Error(fmt.Sprintf("decoding url: %v", err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_, err, _ = h.group.Do(id.String(), func() (interface{}, error) {
		writer, abort, cacheErr := h.cache.Write(id)
		if errors.Is(cacheErr, ErrExists) {
			return nil, nil
		}
		if cacheErr != nil {
			return nil, cacheErr
		}
		defer func() { _ = writer.Close() }()
		_, cacheErr = io.Copy(writer, r.Body)
		if cacheErr != nil {
			abortErr := abort()
			if abortErr != nil {
				return nil, abortErr
			}
			return nil, cacheErr
		}
		return nil, nil
	})
	if err != nil {
		h.logger.Error(fmt.Sprintf("group do: %v", err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/file", h)
}
