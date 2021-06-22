package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/Neurostep/go-nate/internal/dl"
	"github.com/Neurostep/go-nate/internal/dump"
	"github.com/Neurostep/go-nate/internal/indexer"
	"github.com/Neurostep/go-nate/internal/logger"
	"github.com/Neurostep/go-nate/internal/server"
	ua "github.com/Neurostep/go-nate/internal/user-agents"
	"github.com/blevesearch/bleve/v2"
	"github.com/dgraph-io/badger/v3"
	"log"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/konoui/alfred-bookmarks/pkg/bookmarker"
)

var (
	_firefoxProfileName = "default"
	_chromeProfileName  = "default"

	_chromeDataPath = os.ExpandEnv("${HOME}/Library/Application Support/Google/Chrome")
	_firefoxDataPath = os.ExpandEnv("${HOME}/Library/Application Support/Firefox/Profiles")
)

const (
	_chromeBrowser = "chrome"
	_safariBrowser = "safari"
	_firefoxBrowser = "firefox"
)

func main() {
	var (
		logPath, dbPath, indexPath string

		rootFlagSet   = flag.NewFlagSet("go-nate", flag.ExitOnError)
		dumpFlagSet   = flag.NewFlagSet("dump", flag.ExitOnError)
		indexFlagSet  = flag.NewFlagSet("index", flag.ExitOnError)
		watchFlagSet = flag.NewFlagSet("watch", flag.ExitOnError)
		serverFlagSet = flag.NewFlagSet("server", flag.ExitOnError)
	)

	rootFlagSet.StringVar(&logPath, "l", "./log", "Path to directory containing logs")
	rootFlagSet.StringVar(&dbPath, "s", "./db", "Path to directory containing database files")
	rootFlagSet.StringVar(&indexPath, "i", "./index", "Path to directory containing search index")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		log.Fatalf("fatal: couldn't open db %s", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Printf("error: couldn't close db connection %s", err)
		}
	}()

	initBookmarkManager := func(browser, path, profile *string) (bookmarker.Bookmarker, error) {
		var opt bookmarker.Option
		switch *browser {
		case _chromeBrowser:
			if *path == "" {
				*path = _chromeDataPath
			}
			if *profile == "" {
				*profile = _chromeProfileName
			}
			opt = bookmarker.OptionChrome(*path, *profile)
		case _firefoxBrowser:
			if *path == "" {
				*path = _firefoxDataPath
			}
			if *profile == "" {
				*profile = _firefoxProfileName
			}
			opt = bookmarker.OptionFirefox(*path, *profile)
		case _safariBrowser:
			opt = bookmarker.OptionSafari()
		}

		manager, err := bookmarker.New(opt)
		if err != nil {
			return nil, err
		}

		return manager, nil
	}

	var dumpBookmarksPath, dumpBrowser, dumpBrowserProfile string
	var dumpConcurrency int
	var forceDump bool
	dumpFlagSet.StringVar(&dumpBookmarksPath, "f", _chromeDataPath, "The path to local browser profile")
	dumpFlagSet.StringVar(&dumpBrowser, "b", _chromeBrowser, "Browser for which bookmarks are being dumped")
	dumpFlagSet.StringVar(&dumpBrowserProfile, "p", _chromeProfileName, "The profile name of the browser")
	dumpFlagSet.IntVar(&dumpConcurrency, "c", 100, "Number of concurrent workers to dump the bookmarks")
	dumpFlagSet.BoolVar(&forceDump, "F", false, "If provided, then bookmark will be dumped even if it already exists")

	uaStream, err := ua.NewRandomStream()
	if err != nil {
		log.Fatalf("fatal: couldn't initialize ua reader %s", err)
	}
	defer func() {
		err := uaStream.Close()
		if err != nil {
			log.Print(err)
		}
	}()

	d := &ffcli.Command{
		Name:       "dump",
		ShortUsage: "go-nate dump [-f path] [-b browser] [-p profile] [-c concurrency] [-F force to dump] [bookmark url] [bookmark folder] [bookmark title]",
		ShortHelp:  "Saves bookmarks for the specified browser to the local DB. If bookmark URL is provided, it will dump that one only",
		FlagSet:    dumpFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			l, err := logger.New("dump", logPath)
			if err != nil {
				return err
			}

			manager, err := initBookmarkManager(&dumpBrowser, &dumpBookmarksPath, &dumpBrowserProfile)
			if err != nil {
				return err
			}

			httpL := dl.NewHttpLoader()
			chromeL := dl.NewChromeLoader()
			defer chromeL.Stop()

			d, err := dump.NewDump(&dump.Props{
				Bm:              manager,
				Logger:          l,
				PoolSize:        dumpConcurrency,
				UserAgentStream: uaStream,
				HttpLoader:      httpL,
				ChromeLoader:    chromeL,
				Db: db,
			})
			if err != nil {
				return err
			}

			if len(args) > 0 {
				var folder, title string
				if len(args) > 1 {
					folder = args[1]
				}
				if len(args) == 3 {
					title = args[2]
				}
				err = d.DumpBookmark(ctx, dump.DumpRequest{
					Href:          args[0],
					Folder:        folder,
					OriginalTitle: title,
					Force:         forceDump,
				})
				if err != nil {
					l.Error(err)
					return err
				}
			} else {
				err = d.Run(ctx, forceDump)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	i := &ffcli.Command{
		Name:       "index",
		ShortUsage: "go-nate index [bookmark url]",
		ShortHelp:  "Indexes bookmarks from DB. If 'bookmark url' is provided, it will index only that bookmark",
		FlagSet:    indexFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			l, err := logger.New("index", logPath)
			if err != nil {
				return err
			}

			bmIndex, err := bleve.Open(indexPath)
			if err == bleve.ErrorIndexPathDoesNotExist {
				indexMapping, err := indexer.BuildIndexMapping()
				if err != nil {
					l.Errorf("couldn't build index mapping %s", err)
					return err
				}

				bmIndex, err = bleve.New(indexPath, indexMapping)
				if err != nil {
					l.Errorf("couldn't create index %s", err)
					return err
				}

			} else if err != nil {
				l.Errorf("couldn't initialize index %s", err)
				return err
			}
			defer func() {
				err := bmIndex.Close()
				if err != nil {
					l.Error(err)
				}
			}()

			id := indexer.New(bmIndex, db, l)

			if len(args) == 1 {
				err = id.IndexBookmark(args[0])
				if err != nil {
					l.Error(err)
					return err
				}
			} else {
				err = id.IndexBookmarks()
				if err != nil {
					l.Error(err)
					return err
				}
			}

			return nil
		},
	}

	var watchInterval time.Duration
	var watchBookmarksPath, watchBrowser, watchBrowserProfile string
	watchFlagSet.DurationVar(&watchInterval, "i", time.Second * 30, "The interval in which watch will perform the bookmark file check")
	watchFlagSet.StringVar(&watchBookmarksPath, "f", _chromeDataPath, "The path to local browser profile")
	watchFlagSet.StringVar(&watchBrowser, "b", _chromeBrowser, "Browser for which bookmarks are being watched and dumped")
	watchFlagSet.StringVar(&watchBrowserProfile, "p", _chromeProfileName, "The profile name of the browser")

	w := &ffcli.Command{
		Name: "watch",
		ShortUsage: "go-nate watch [-i interval] [-f path] [-b browser] [-p profile]",
		ShortHelp: "Runs a background check for the bookmark file change",
		FlagSet: watchFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			dumpLogger, err := logger.New("dump", logPath)
			if err != nil {
				return err
			}

			indexLogger, err := logger.New("index", logPath)
			if err != nil {
				return err
			}

			watchLogger, err := logger.New("watch", logPath)
			if err != nil {
				return err
			}

			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return err
			}
			defer func() {
				err := watcher.Close()
				watchLogger.Error(err)
			}()

			manager, err := initBookmarkManager(&watchBrowser, &watchBookmarksPath, &watchBrowserProfile)
			if err != nil {
				return err
			}

			httpL := dl.NewHttpLoader()
			chromeL := dl.NewChromeLoader()
			defer chromeL.Stop()

			d, err := dump.NewDump(&dump.Props{
				Bm:              manager,
				Logger:          dumpLogger,
				PoolSize:        dumpConcurrency,
				UserAgentStream: uaStream,
				HttpLoader:      httpL,
				ChromeLoader:    chromeL,
				Db: db,
			})
			if err != nil {
				return err
			}

			bmIndex, err := bleve.Open(indexPath)
			defer func() {
				err := bmIndex.Close()
				if err != nil {
					indexLogger.Error(err)
				}
			}()

			id := indexer.New(bmIndex, db, indexLogger)

			errs := make(chan error)
			done := make(chan bool)
			defer func() {
				close(errs)
				close(done)
			}()

			go func() {
				tick := time.Tick(watchInterval)
				var evs int
			Loop:
				for {
					select {
					case ev, ok := <-watcher.Events:
						if !ok {
							done <- true
						}
						if ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
							evs++
						}
					case err, ok := <-watcher.Errors:
						if !ok {
							done <- true
						}
						if err != nil {
							watchLogger.Error(err)
						}
					case <-tick:
						if evs == 0 {
							continue
						}
						evs = 0
						err := d.Run(ctx, false)
						if err != nil {
							errs <- err
							break Loop
						}

						err = id.IndexBookmarks()
						if err != nil {
							errs <- err
							break Loop
						}

					case <-ctx.Done():
						done <- true
						break Loop
					}
				}
			}()

			var bmFile string
			switch watchBrowser {
			case _chromeBrowser:
				bmFile, err = bookmarker.GetChromeBookmarkFile(watchBookmarksPath, watchBrowserProfile)
				if err != nil {
					return err
				}
			case _firefoxBrowser:
				bmFile, err = bookmarker.GetFirefoxBookmarkFile(watchBookmarksPath, watchBrowserProfile)
				if err != nil {
					return err
				}
			case _safariBrowser:
				bmFile, err = bookmarker.GetSafariBookmarkFile()
				if err != nil {
					return err
				}
			}

			err = watcher.Add(bmFile)
			if err != nil {
				return err
			}

			select {
			case err := <-errs:
				return err
			case <-done:
			}

			return nil
		},
	}

	var serverPort int
	serverFlagSet.IntVar(&serverPort, "p", 8080, "Number represents the port server will listen to")
	s := &ffcli.Command{
		Name:       "server",
		ShortUsage: "go-nate server [-p port]",
		ShortHelp:  "Runs HTTP server on provided port",
		FlagSet:    serverFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			l, err := logger.New("server", logPath)
			if err != nil {
				return err
			}

			bmIndex, err := bleve.Open(indexPath)
			if err != nil {
				l.Errorf("couldn't open index %s", err)
				return err
			}

			srv := server.New(server.Props{
				Port:   serverPort,
				Logger: l,
				Index:  bmIndex,
			})

			err = srv.Run(ctx)
			if err != nil {
				l.Errorf("server error: %s", err)
				return err
			}

			return nil
		},
	}

	root := &ffcli.Command{
		ShortUsage:  "go-nate [flags] <command> [<args>]",
		Subcommands: []*ffcli.Command{d, i, w, s},
		FlagSet:     rootFlagSet,
		UsageFunc:   DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}

	err = root.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("fatal: couldn't parse CLI arguments %s", err)
	}

	err = root.Run(ctx)
	if err != nil {
		log.Fatalf("fatal: go-nate has failed %s", err)
	}

	select {
	case <-ctx.Done():
		stop()
		log.Fatalf("go-nate interrupted: %s", ctx.Err())
	default:
		log.Println("go-nate finished")
	}
}

func DefaultUsageFunc(c *ffcli.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Usage: ")
	if c.ShortUsage != "" {
		fmt.Fprintf(&b, "%s\n", c.ShortUsage)
	} else {
		fmt.Fprintf(&b, "%s\n", c.Name)
	}
	fmt.Fprintf(&b, "\n")

	if c.LongHelp != "" {
		fmt.Fprintf(&b, "%s\n\n", c.LongHelp)
	}

	if len(c.Subcommands) > 0 {
		fmt.Fprintf(&b, "Commands:\n")
		tw := tabwriter.NewWriter(&b, 0, 4, 4, ' ', 0)
		for _, subcommand := range c.Subcommands {
			fmt.Fprintf(tw, "\t%s\t%s\n", subcommand.Name, subcommand.ShortHelp)
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.FlagSet) > 0 {
		fmt.Fprintf(&b, "Flags:\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		c.FlagSet.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(tw, "  --%s\t%s\n", f.Name, f.Usage)
		})
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	return strings.TrimSpace(b.String())
}

func countFlags(fs *flag.FlagSet) (n int) {
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n
}
