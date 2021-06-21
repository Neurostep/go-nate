package user_agents

import (
	"embed"
	"encoding/csv"
	"io"
	"io/fs"
	"math/rand"
	"sync"
	"time"
)

type (
	RandomStream struct {
		mu   sync.Mutex
		fh   fs.File
		rd   *csv.Reader
		ln   int
		pick string
	}
)

//go:embed user-agents.csv
var uaFile embed.FS

func NewRandomStream() (*RandomStream, error) {
	fh, err := uaFile.Open("user-agents.csv")
	if err != nil {
		return nil, err
	}

	csvr := csv.NewReader(fh)

	return &RandomStream{fh: fh, ln: 1, rd: csvr}, nil
}

func (r *RandomStream) Get() (string, error) {
	randsource := rand.NewSource(time.Now().UnixNano())
	randgenerator := rand.New(randsource)

	r.mu.Lock()
	defer r.mu.Unlock()

	for {
		row, err := r.rd.Read()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return "", err
		}

		// skip first row
		if row[0] == "id" {
			continue
		}

		roll := randgenerator.Intn(r.ln)
		if roll == 0 {
			r.pick = row[1]
		}

		r.ln += 1
		break
	}

	return r.pick, nil
}

func (r *RandomStream) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ln = 1
	r.pick = ""

	return r.fh.Close()
}
