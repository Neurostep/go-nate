package dl

import (
	"context"
	"net/http"
)

type (
	HttpInstance struct {
		cfg config
	}
)

func NewHttpLoader(options ...Option) *HttpInstance {
	cfg := config{
		UserAgent: DefaultUA,
	}
	for _, opt := range options {
		opt(&cfg)
	}

	return &HttpInstance{cfg: cfg}
}

func (hi *HttpInstance) Get(ctx context.Context, url string, ua string) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Accept-Charset","utf-8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ru;q=0.8")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
