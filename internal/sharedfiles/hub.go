package sharedfiles

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Hub is the server's blobs as a worker reaches them, with its credential.
type Hub struct {
	Server     string
	Credential string
}

// BlobsPath is where a worker reaches the server's blobs.
const BlobsPath = "/api/worker/blobs"

func (h Hub) request(method, path string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, strings.TrimSuffix(h.Server, "/")+BlobsPath+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+h.Credential)
	return http.DefaultClient.Do(request)
}

func (h Hub) Has(hash string) bool {
	response, err := h.request(http.MethodHead, "/"+hash, nil)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (h Hub) Open(hash string) (io.ReadCloser, error) {
	response, err := h.request(http.MethodGet, "/"+hash, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("getting a shared file from the server: %s", response.Status)
	}
	return response.Body, nil
}

func (h Hub) Put(r io.Reader) (string, int64, error) {
	response, err := h.request(http.MethodPost, "", r)
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		said, _ := io.ReadAll(response.Body)
		return "", 0, fmt.Errorf("sending a shared file to the server: %s", strings.TrimSpace(string(said)))
	}
	var kept struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	}
	return kept.Hash, kept.Size, json.NewDecoder(response.Body).Decode(&kept)
}
