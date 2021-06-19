package user_agents

import (
	"encoding/csv"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"
)

type (
	RandomStream struct {
		mu   sync.Mutex
		fh   *os.File
		rd   *csv.Reader
		ln   int
		pick string
	}
)

func NewRandomStream(path string) (*RandomStream, error) {
	fh, err := os.OpenFile(path, os.O_RDONLY, 0644)
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
