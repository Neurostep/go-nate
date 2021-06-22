package dump

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Neurostep/go-nate/internal/dl"
	"github.com/Neurostep/go-nate/internal/indexer"
	"github.com/Neurostep/go-nate/internal/pool"
	ua "github.com/Neurostep/go-nate/internal/user-agents"
	rw "github.com/Neurostep/readability-wrapper-go/readabilitywrapper"
	"github.com/aws/jsii-runtime-go"
	"github.com/dgraph-io/badger/v3"
	"github.com/konoui/alfred-bookmarks/pkg/bookmarker"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"net/http"

	"github.com/abadojack/whatlanggo"

	"github.com/cloudflare/backoff"
	"go.uber.org/ratelimit"
	"io/ioutil"
	"net/url"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
)

type (
	Props struct {
		Logger          *zap.SugaredLogger
		PoolSize        int
		HttpLoader      *dl.HttpInstance
		ChromeLoader    *dl.ChromeInstance
		Bm              bookmarker.Bookmarker
		UserAgentStream *ua.RandomStream
		Db *badger.DB
	}

	Dump struct {
		mux     sync.Mutex
		p       *pool.Pool
		l       *zap.SugaredLogger
		r       rw.ReadabilityWrapper
		bm      bookmarker.Bookmarker
		db *badger.DB
		httpL   *dl.HttpInstance
		chromeL *dl.ChromeInstance
		ua      *ua.RandomStream
	}

	DumpRequest struct {
		Href, Folder, OriginalTitle string
		Force                       bool
	}
)

const (
	defaultRateLimit   = 2
	backoffMaxDuration = time.Minute * 1
	backoffInterval    = time.Second * 10
	backoffMaxAttempts = 3
)

func NewDump(props *Props) (*Dump, error) {
	p := pool.NewPool(props.PoolSize)

	return &Dump{
		p:       p,
		ua:      props.UserAgentStream,
		l:       props.Logger,
		bm:      props.Bm,
		db: props.Db,
		httpL:   props.HttpLoader,
		chromeL: props.ChromeLoader,
		r:       rw.NewReadabilityWrapper(&rw.ReadabilityProps{Name: jsii.String("a")}),
	}, nil
}

func (d *Dump) Parse(body, href string) *rw.ReadabilityResult {
	d.mux.Lock()
	defer d.mux.Unlock()

	return d.r.Parse(jsii.String(body), jsii.String(href))
}

func (d *Dump) Run(ctx context.Context, force bool) error {
	var wg sync.WaitGroup
	var hostBuckets = map[string]ratelimit.Limiter{}

	r, err := d.bm.Bookmarks()
	if err != nil {
		return err
	}

	pBar := pb.StartNew(len(r))

	for _, b := range r {
		bookmarkExist, err := d.Exists(b.URI)
		if err != nil {
			return err
		}

		if !force && bookmarkExist {
			pBar.Increment()
			continue
		}

		wg.Add(1)

		parsedUrl, err := url.Parse(b.URI)
		if err != nil {
			d.l.Errorf("couldn't parse URL: %s", b.URI)
			return err
		}

		_, ok := hostBuckets[parsedUrl.Host]
		if !ok {
			hostBuckets[parsedUrl.Host] = ratelimit.New(defaultRateLimit)
		}
		rl := hostBuckets[parsedUrl.Host]

		func(b *bookmarker.Bookmark) {
			d.p.Schedule(func() {
				defer func() {
					wg.Done()
					pBar.Increment()
					if x := recover(); x != nil {
						d.l.Errorf("run time panic: %v. HREF: %s", x, b.URI)
					}
				}()

				rl.Take()

				err := d.DumpBookmark(ctx, DumpRequest{
					Href:          b.URI,
					Folder:        b.Folder,
					OriginalTitle: b.Title,
					Force:         force,
				})
				if err != nil {
					d.l.Errorf("failed dumping bookmark: %s", err)
				}
			})
		}(b)
	}

	wg.Wait()
	pBar.Finish()

	return nil
}

func (d *Dump) DumpBookmark(ctx context.Context, req DumpRequest) error {
	bookmarkExist, err := d.Exists(req.Href)
	if err != nil {
		return err
	}

	if !req.Force && bookmarkExist {
		return nil
	}

	attempts := 0

	back := backoff.New(backoffMaxDuration, backoffInterval)
	defer back.Reset()

	var body []byte

	for {
		attempts++

		uaString, err := d.ua.Get()
		if err != nil {
			return errors.Wrap(err, "couldn't get User Agent")
		}

		r, err := d.httpL.Get(ctx, req.Href, uaString)
		if err != nil {
			return err
		}

		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("couldn't read response body for HREF: %s", req.Href))
		}

		if r.StatusCode != http.StatusOK {
			if (r.StatusCode == http.StatusForbidden || r.StatusCode == http.StatusServiceUnavailable) && attempts < backoffMaxAttempts {
				nextTry := back.Duration()
				<-time.After(nextTry)
				continue
			}
			d.l.Debugf("received non-200 HTTP code: %d. HREF: %s, attempts: %d, trying the chrome...", r.StatusCode, req.Href, attempts)

			chromeResp, err := d.chromeL.Get(ctx, req.Href)
			if err != nil {
				return errors.Wrap(err, "got error while using chrome")
			}

			body, err = ioutil.ReadAll(chromeResp.Body)
			if err != nil {
				return errors.Wrapf(err, "couldn't read response body for HREF: %s", req.Href)
			}
		}

		if body == nil {
			return errors.Errorf("body is nil for HREF: %s. Status is %s", req.Href, r.Status)
		}

		if attempts > 1 {
			d.l.Debugf("able to re-retrieve %s page after %d attempts", req.Href, attempts)
		}

		break
	}

	pr := d.Parse(string(body), req.Href)

	var title, html, text, excerpt, author, site string
	if pr.Title != nil {
		title = *pr.Title
		if title == "" {
			title = req.OriginalTitle
		}
	}
	if pr.Content != nil {
		html = *pr.Content
	}
	if pr.TextContent != nil {
		text = *pr.TextContent
	}
	if pr.Excerpt != nil {
		excerpt = *pr.Excerpt
	}
	if pr.Byline != nil {
		author = *pr.Byline
	}
	if pr.SiteName != nil {
		site = *pr.SiteName
	}

	var lang string
	langSpecificFields := []string{text, excerpt, title}
	for _, t := range langSpecificFields {
		if t != "" {
			langInfo := whatlanggo.Detect(t)
			if langInfo.Confidence > 0.95 {
				lang = whatlanggo.LangToStringShort(langInfo.Lang)
			} else {
				lang = whatlanggo.LangToStringShort(whatlanggo.Eng)
			}
		}
	}

	_, langSupported := indexer.SupportedLanguages[lang]
	if lang == "" || !langSupported {
		lang = whatlanggo.LangToStringShort(whatlanggo.Eng)
	}

	bmJson := map[string]string{
		fmt.Sprintf("%s_title", lang):   title,
		fmt.Sprintf("%s_html", lang):    html,
		fmt.Sprintf("%s_text", lang):    text,
		fmt.Sprintf("%s_excerpt", lang): excerpt,
		"lang":                          lang,
		"author":                        author,
		"siteName":                      site,
		"url":                           req.Href,
		"folder":                        req.Folder,
	}

	err = d.Save(bmJson)

	return errors.Wrapf(err, "couldn't save file %s", *pr.Title)
}

func (d *Dump) Save(b map[string]string) error {
	return d.db.Update(func(txn *badger.Txn) error {
		bm, err := json.Marshal(&b)
		if err != nil {
			return err
		}

		return txn.Set([]byte(b["url"]), bm)
	})
}

func (d *Dump) Exists(href string) (bool, error) {
	var exists bool

	err := d.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(href))
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		exists = err != badger.ErrKeyNotFound

		return nil
	})

	return exists, err
}
