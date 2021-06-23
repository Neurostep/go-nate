package server

import (
	"context"
	"embed"
	"fmt"
	"github.com/Neurostep/go-nate/internal/logger"
	"github.com/blevesearch/bleve/v2"
	bleveHttp "github.com/blevesearch/bleve/v2/http"
	"github.com/gorilla/mux"
	"net/http"
	"time"
)

type (
	Props struct {
		Port   int
		Logger *logger.Logger
		Index  bleve.Index
	}

	server struct {
		http.Server
		l *logger.Logger
		i bleve.Index
	}
)

//go:embed static
var serverStaticFiles embed.FS

func New(props Props) *server {
	s := &server{
		Server: http.Server{
			Addr:         fmt.Sprintf(":%d", props.Port),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		l: props.Logger,
		i: props.Index,
	}

	router := staticFileRouter()

	// add the API
	bleveHttp.RegisterIndexName("bookmark", s.i)
	searchHandler := bleveHttp.NewSearchHandler("bookmark")
	router.Handle("/api/search", searchHandler).Methods("POST")

	listFieldsHandler := bleveHttp.NewListFieldsHandler("bookmark")
	router.Handle("/api/fields", listFieldsHandler).Methods("GET")

	debugHandler := bleveHttp.NewDebugDocumentHandler("bookmark")
	debugHandler.DocIDLookup = docIDLookup
	router.Handle("/api/debug/{docID}", debugHandler).Methods("GET")

	s.Handler = router

	http.Handle("/", router)

	return s
}

func (s *server) Run(ctx context.Context) error {
	errCh := make(chan error)

	s.l.Infof("starting server to listen %s...", s.Addr)
	go func() {
		err := s.ListenAndServe()
		if err != nil {
			s.l.Errorf("error starting server %s", err)
		}
		errCh <- err
	}()

	var err error

	select {
	case <-ctx.Done():
		ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		s.l.Info("shutting down the server...")
		err := s.Shutdown(ctxTimeout)
		if err != nil {
			errCh <- err
		}
	case err = <-errCh:
	}

	return err
}

func staticFileRouter() *mux.Router {
	r := mux.NewRouter()
	r.StrictSlash(true)

	var static = http.FS(serverStaticFiles)

	// static
	r.PathPrefix("/static/").Handler(http.StripPrefix("/",
		myFileHandler{http.FileServer(static)}))

	// application pages
	appPages := []string{
		"/search",
	}

	for _, p := range appPages {
		// if you try to use index.html it will redirect...poorly
		r.PathPrefix(p).Handler(RewriteURL("/static/",
			http.FileServer(static)))
	}

	r.Handle("/", http.RedirectHandler("/static/index.html", 302))

	return r
}

type myFileHandler struct {
	h http.Handler
}

func (mfh myFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mfh.h.ServeHTTP(w, r)
}

func RewriteURL(to string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = to
		h.ServeHTTP(w, r)
	})
}

func muxVariableLookup(req *http.Request, name string) string {
	return mux.Vars(req)[name]
}

func docIDLookup(req *http.Request) string {
	return muxVariableLookup(req, "docID")
}
