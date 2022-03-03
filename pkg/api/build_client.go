// +build !solution

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"gitlab.com/slon/shad-go/distbuild/pkg/build"
)

type BuildClient struct {
	logger   *zap.Logger
	endpoint string
}

func NewBuildClient(l *zap.Logger, endpoint string) *BuildClient {
	return &BuildClient{
		logger:   l.Named("build client"),
		endpoint: endpoint,
	}
}

type JSONStatusReader struct {
	reader  io.ReadCloser
	logger  *zap.Logger
	decoder *json.Decoder
	closed  bool
}

func (r *JSONStatusReader) Close() error {
	if !r.closed {
		r.closed = true
		return r.reader.Close()
	}
	return errors.New("closed called second time")
}

func (r *JSONStatusReader) Next() (*StatusUpdate, error) {
	if r.closed {
		return nil, io.EOF
	}
	var su StatusUpdate
	if err := r.decoder.Decode(&su); err != nil {
		r.logger.Error(fmt.Sprintf("error decoding message: %v", err))
		return nil, err
	}
	return &su, nil
}

func (c *BuildClient) StartBuild(ctx context.Context, request *BuildRequest) (*BuildStarted, StatusReader, error) {
	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		c.logger.Error(fmt.Sprintf("encoding request: %v", err))
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/build", &buf)
	if err != nil {
		c.logger.Error(fmt.Sprintf("creating http request: %v", err))
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.logger.Error(fmt.Sprintf("getting response: %v", err))
		return nil, nil, err
	}
	decoder := json.NewDecoder(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		defer func() { _ = httpResp.Body.Close() }()
		var update StatusUpdate
		err = decoder.Decode(&update)
		c.logger.Info(fmt.Sprintf("status error, %s", http.StatusText(httpResp.StatusCode)))
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New(update.BuildFailed.Error)
	}
	var started BuildStarted
	if err := decoder.Decode(&started); err != nil {
		c.logger.Error(fmt.Sprintf("decoding started: %v", err))
		_ = httpResp.Body.Close()
		return nil, nil, err
	}
	var statusReader StatusReader = &JSONStatusReader{
		reader:  httpResp.Body,
		logger:  c.logger.Named("json status reader"),
		decoder: decoder,
		closed:  false,
	}
	return &started, statusReader, nil
}

func (c *BuildClient) SignalBuild(ctx context.Context, buildID build.ID, signal *SignalRequest) (*SignalResponse, error) {
	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(signal); err != nil {
		c.logger.Error(fmt.Sprintf("encoding request: %v", err))
		return nil, err
	}
	uri, err := url.Parse(c.endpoint + "/signal")
	if err != nil {
		panic(err)
	}
	q := uri.Query()
	q.Set("build_id", buildID.String())
	uri.RawQuery = q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), &buf)
	if err != nil {
		c.logger.Error(fmt.Sprintf("creating http request: %v", err))
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.logger.Error(fmt.Sprintf("getting response: %v", err))
		return nil, err
	}
	defer func() { _ = httpResp.Body.Close() }()
	if httpResp.StatusCode != http.StatusOK {
		c.logger.Info(fmt.Sprintf("status error: %s", http.StatusText(httpResp.StatusCode)))
		errString, err := ioutil.ReadAll(httpResp.Body)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(string(errString))
	}
	var signalResp SignalResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&signalResp); err != nil {
		c.logger.Error(fmt.Sprintf("decoding signal: %v", err))
		return nil, err
	}
	return &signalResp, nil
}
