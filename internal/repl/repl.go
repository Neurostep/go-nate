package repl

import (
	"fmt"
	"github.com/blevesearch/bleve/v2"
	"github.com/peterh/liner"
	"github.com/pkg/errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type (
	Repl struct {
		out, err   io.Writer
		index      bleve.Index
		historyDir string
		settings   map[string]map[string]interface{}
	}
)

var (
	_errUnknownCommand   = errors.New("unknown command")
	_errUnknownSetting   = errors.New("unknown setting")
	_errWrongNumericType = errors.New("expected numeric value type")
)

const (
	_promptDefault = "go-nate> "
	_searchCommand = "search"
	_setCommand    = "set"
)

func New(index bleve.Index, historyDir string) *Repl {
	settings := map[string]map[string]interface{}{
		"search": {
			"searchResultSize": 10,
		},
	}

	return &Repl{
		out:        os.Stdout,
		err:        os.Stderr,
		index:      index,
		historyDir: historyDir,
		settings:   settings,
	}
}

func (r *Repl) Run() error {
	rl := liner.NewLiner()
	rl.SetCtrlCAborts(true)
	defer rl.Close()

	var historyFile string

	historyFile = filepath.Join(r.historyDir, "history")

	f, err := os.Open(historyFile)
	if err != nil {
		if !os.IsNotExist(err) {
			r.errorf("%s", err)
		}
	} else {
		_, err := rl.ReadHistory(f)
		if err != nil {
			r.errorf("while reading history: %s", err)
		}
		f.Close()
	}

	for {
		in, err := rl.Prompt(_promptDefault)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(r.err, "fatal: %s", err)
			os.Exit(1)
		}

		if in == "" {
			continue
		}

		err = r.handleInput(in)
		if err != nil {
			r.errorf("can not process input, %s", err)
			continue
		}
		rl.AppendHistory(in)
	}

	if historyFile != "" {
		err := os.MkdirAll(filepath.Dir(historyFile), 0o755)
		if err != nil {
			r.errorf("%s", err)
		} else {
			f, err := os.Create(historyFile)
			if err != nil {
				r.errorf("%s", err)
			} else {
				_, err := rl.WriteHistory(f)
				if err != nil {
					r.errorf("while saving history: %s", err)
				}
				f.Close()
			}
		}
	}

	return nil
}

func (r *Repl) handleInput(in string) error {
	ins := strings.Split(in, " ")
	cmd := ins[0]

	switch cmd {
	case _searchCommand:
		return r.handleSearch(strings.Join(ins[1:], " "))
	case _setCommand:
		return r.handleSetting(ins[1], ins[2], ins[3])
	}

	return _errUnknownCommand
}

func (r *Repl) handleSetting(command, name string, value interface{}) error {
	if _, ok := r.settings[command]; !ok {
		return _errUnknownCommand
	}

	if _, ok := r.settings[command][name]; !ok {
		return _errUnknownSetting
	}

	var val int
	switch value.(type) {
	case int, int32, int64, float32, float64:
		val = value.(int)
	case string:
		v, err := strconv.Atoi(value.(string))
		if err != nil {
			return err
		}
		val = v
	default:
		return _errWrongNumericType
	}

	r.settings[command][name] = val

	return nil
}

func (r *Repl) handleSearch(search string) error {
	searchRequest := bleve.NewSearchRequestOptions(
		bleve.NewQueryStringQuery(search), r.settings["search"]["searchResultSize"].(int), 0, false)
	searchRequest.Fields = []string{"*"}

	res, err := r.index.Search(searchRequest)
	if err != nil {
		return err
	}

	var rv string
	if res.Total > 0 {
		if res.Request.Size > 0 {
			rv = fmt.Sprintf("%d matches, showing %d through %d, took %s\n", res.Total, res.Request.From+1, res.Request.From+len(res.Hits), res.Took)
			for i, hit := range res.Hits {
				lang := hit.Fields["lang"]
				rv += fmt.Sprintf(
					"%5d. %s - %s (%f)\n", i+res.Request.From+1, hit.Fields[fmt.Sprintf("%s_title", lang)].(string), hit.Fields["url"].(string), hit.Score)
			}
		} else {
			rv = fmt.Sprintf("%d matches, took %s\n", res.Total, res.Took)
		}
	}

	_, err = fmt.Fprintf(r.out, rv)

	return err
}

func (r *Repl) errorf(format string, args ...interface{}) {
	fmt.Fprintf(r.err, "error: "+format+"\n", args...)
}
