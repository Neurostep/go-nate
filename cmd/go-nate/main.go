package main

import (
	"context"
	"flag"
	"fmt"
	bookmarks_manager "github.com/Neurostep/go-nate/internal/bookmarks-manager"
	"github.com/Neurostep/go-nate/internal/dl"
	"github.com/Neurostep/go-nate/internal/dump"
	"github.com/Neurostep/go-nate/internal/indexer"
	"github.com/Neurostep/go-nate/internal/logger"
	"github.com/Neurostep/go-nate/internal/server"
	user_agents "github.com/Neurostep/go-nate/internal/user-agents"
	"github.com/blevesearch/bleve/v2"
	"github.com/dgraph-io/badger/v3"
	"log"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"

	"github.com/peterbourgon/ff/v3/ffcli"
)

func main() {
	var (
		logPath, dbPath, indexPath, uaPath string

		rootFlagSet   = flag.NewFlagSet("go-nate", flag.ExitOnError)
		dumpFlagSet   = flag.NewFlagSet("dump", flag.ExitOnError)
		indexFlagSet  = flag.NewFlagSet("index", flag.ExitOnError)
		serverFlagSet = flag.NewFlagSet("server", flag.ExitOnError)
	)

	rootFlagSet.StringVar(&logPath, "l", "./log", "Path to directory containing logs")
	rootFlagSet.StringVar(&dbPath, "s", "./db", "Path to directory containing database files")
	rootFlagSet.StringVar(&indexPath, "i", "./index", "Path to directory containing search index")
	rootFlagSet.StringVar(&uaPath, "u", "./user-agents.csv", "Path to file with user agents in csv format")

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

	var bookmarksFile string
	var dumpConcurrency int
	var forceDump bool
	dumpFlagSet.StringVar(&bookmarksFile, "f", "bookmarks.json", "The path to local JSON file containing the bookmarks")
	dumpFlagSet.IntVar(&dumpConcurrency, "c", 100, "Number of concurrent workers to dump the bookmarks")
	dumpFlagSet.BoolVar(&forceDump, "F", false, "If provided, then bookmark will be dumped even if it already exists")

	l, err := logger.New("dump", logPath)
	if err != nil {
		log.Fatal(err)
	}
	bm, err := bookmarks_manager.NewManager(bookmarks_manager.Props{
		FilePath: bookmarksFile,
		Db:       db,
		Logger:   l,
	})
	if err != nil {
		log.Fatalf("fatal: couldn't initialize bookamrks manager %s", err)
	}

	uaStream, err := user_agents.NewRandomStream(uaPath)
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
		ShortUsage: "go-nate dump [-f path] [-c concurrency] [-F force to dump] [bookmark url] [bookmark folder] [bookmark title]",
		ShortHelp:  "Saves bookmarks from the specified JSON file to the local DB. If bookmark URL is provided, it will dump that one only",
		FlagSet:    dumpFlagSet,
		Exec: func(ctx context.Context, args []string) error {
			l, err := logger.New("dump", logPath)
			if err != nil {
				return err
			}

			httpL := dl.NewHttpLoader()
			chromeL := dl.NewChromeLoader()
			defer chromeL.Stop()

			d, err := dump.NewDump(&dump.Props{
				Bm:              bm,
				Logger:          l,
				PoolSize:        dumpConcurrency,
				UserAgentStream: uaStream,
				HttpLoader:      httpL,
				ChromeLoader:    chromeL,
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

	var serverPort int
	dumpFlagSet.IntVar(&serverPort, "p", 8080, "Number represents the port server will listen to")
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
		Subcommands: []*ffcli.Command{d, i, s},
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
		log.Fatalf("fatal: run has failed %s", err)
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
