package httpclient

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	MaxJSONResponseBytes    int64 = 64 << 20
	MaxCatalogResponseBytes int64 = 256 << 20
)

func New() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func RedactErrorURL(err error) error {
	if err == nil {
		return nil
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return fmt.Errorf("%s request failed: %w", urlError.Op, urlError.Err)
	}
	return err
}

func ReadAllLimit(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("response size limit must be positive")
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d byte limit", limit)
	}
	return data, nil
}
