package dl

import (
	"bytes"
	"context"
	"github.com/chromedp/cdproto/network"
	"io"
	"net/http"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

const (
	_statusErrorTmpl = "failed to retrieve page, status code %d"

	DefaultUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.93 Safari/537.36"
)

type ChromeInstance struct {
	cfg config

	ctx context.Context // context with the browser

	allocFn   context.CancelFunc // allocator cancel func
	browserFn context.CancelFunc // browser cancel func
	lnCancel  context.CancelFunc // listener cancel func

	mu sync.Mutex
}

type runnerFn = func(ctx context.Context, actions ...chromedp.Action) (*network.Response, error)

// to be able to mock in tests.
var runner runnerFn = chromedp.RunResponse

func NewChromeLoader(options ...Option) *ChromeInstance {
	cfg := config{
		UserAgent: DefaultUA,
	}
	for _, opt := range options {
		opt(&cfg)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(cfg.UserAgent),
	)

	allocCtx, aCancel := chromedp.NewExecAllocator(context.Background(), opts[:]...)
	ctx, cCancel := chromedp.NewContext(allocCtx)

	return newChromeInstance(ctx, cfg, aCancel, cCancel)
}

func newChromeInstance(ctx context.Context, cfg config, allocCFn, ctxCFn context.CancelFunc) *ChromeInstance {
	bi := ChromeInstance{
		cfg: cfg,

		ctx:       ctx,
		allocFn:   allocCFn,
		browserFn: ctxCFn,
	}

	return &bi
}

func (bi *ChromeInstance) Stop() {
	bi.stopListener()

	if bi.allocFn != nil {
		bi.browserFn()
	}
	if bi.allocFn != nil {
		bi.allocFn()
	}
}

func (bi *ChromeInstance) stopListener() {
	if bi.lnCancel == nil {
		return
	}
	bi.mu.Lock()
	defer bi.mu.Unlock()

	bi.lnCancel()
	bi.lnCancel = nil
}

func (bi *ChromeInstance) download(ctx context.Context, uri string) (*bytes.Buffer, error) {
	var str string
	if err := bi.navigate(ctx, uri, &str); err != nil {
		return nil, err
	}

	return bytes.NewBuffer([]byte(str)), nil
}

func (bi *ChromeInstance) Get(ctx context.Context, url string) (*http.Response, error) {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	return bi.get(ctx, url)
}

func (bi *ChromeInstance) get(ctx context.Context, url string) (*http.Response, error) {
	buf, err := bi.download(ctx, url)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("GET", url, nil)
	resp := http.Response{
		Status:        http.StatusText(http.StatusOK),
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Body:          io.NopCloser(buf),
		ContentLength: int64(buf.Len()),
		Close:         true,
		Uncompressed:  true,
		Request:       req,
	}
	return &resp, nil
}

func (bi *ChromeInstance) navigate(ctx context.Context, uri string, str *string) error {
	var errC = make(chan error, 1)

	go func() {
		resp, err := runner(bi.ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
				if err != nil {
					return err
				}
				return nil
			}),
			chromedp.Navigate(uri),
			chromedp.OuterHTML(`html`, str, chromedp.ByQuery),
		)
		if err != nil {
			errC <- err
			return
		}

		if resp.Status != http.StatusOK {
			errC <- errors.Errorf(_statusErrorTmpl, resp.Status)

			return
		}
		errC <- nil
	}()

	select {
	case err := <-errC:
		if err != nil {
			return errors.WithStack(err)
		}
	case <-bi.ctx.Done():
		return errors.WithStack(bi.ctx.Err())
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	}

	return nil
}
