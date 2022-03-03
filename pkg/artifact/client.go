// +build !solution

package artifact

import (
	"context"
	"errors"
	"gitlab.com/slon/shad-go/distbuild/pkg/tarstream"
	"io/ioutil"
	"net/http"
	"net/url"

	"gitlab.com/slon/shad-go/distbuild/pkg/build"
)

// Download artifact from remote cache into local cache.
func Download(ctx context.Context, endpoint string, c *Cache, artifactID build.ID) error {
	uri, err := url.Parse(endpoint + "/artifact")
	if err != nil {
		return err
	}
	q := uri.Query()
	q.Set("id", artifactID.String())
	uri.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(body))
	}
	path, commit, abort, err := c.Create(artifactID)
	if err != nil {
		return err
	}
	err = tarstream.Receive(path, resp.Body)
	if err != nil {
		curErr := abort()
		if curErr != nil {
			return curErr
		}
		return err
	}
	return commit()
}
