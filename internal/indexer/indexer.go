package indexer

import (
	"encoding/json"
	"github.com/Neurostep/go-nate/internal/logger"
	"github.com/blevesearch/bleve/v2"
	"github.com/dgraph-io/badger/v3"
	"time"
)

type (
	Indexer struct {
		i  bleve.Index
		db *badger.DB
		l  *logger.Logger
	}
)

const (
	batchSize = 100
)

func New(i bleve.Index, db *badger.DB, l *logger.Logger) *Indexer {
	return &Indexer{
		i:  i,
		db: db,
		l:  l,
	}
}

func (idx *Indexer) IndexBookmark(href string) error {
	err := idx.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(href))
		if err != nil {
			return err
		}

		var valCopy []byte
		err = item.Value(func(val []byte) error {
			valCopy = append([]byte{}, val...)

			return nil
		})
		if err != nil {
			return err
		}

		var jsonDoc map[string]string
		err = json.Unmarshal(valCopy, &jsonDoc)
		if err != nil {
			return err
		}
		// do not index html
		for k, _ := range SupportedLanguages {
			delete(jsonDoc, k+"_html")
		}

		err = idx.i.Index(href, jsonDoc)
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (idx *Indexer) IndexBookmarks() error {
	count := 0
	startTime := time.Now()
	batch := idx.i.NewBatch()
	batchCount := 0

	err := idx.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = batchSize

		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				var jsonDoc map[string]string
				err := json.Unmarshal(v, &jsonDoc)
				if err != nil {
					return err
				}

				// do not index html
				for k, _ := range SupportedLanguages {
					delete(jsonDoc, k+"_html")
				}

				err = batch.Index(string(k), jsonDoc)
				if err != nil {
					return err
				}

				batchCount++

				if batchCount >= batchSize {
					err = idx.i.Batch(batch)
					if err != nil {
						return err
					}
					batch = idx.i.NewBatch()
					batchCount = 0
				}
				count++
				if count%1000 == 0 {
					indexDuration := time.Since(startTime)
					indexDurationSeconds := float64(indexDuration) / float64(time.Second)
					timePerDoc := float64(indexDuration) / float64(count)
					idx.l.Infof("Indexed %d documents, in %.2fs (average %.2fms/doc)", count, indexDurationSeconds, timePerDoc/float64(time.Millisecond))
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if batchCount > 0 {
		err = idx.i.Batch(batch)
		if err != nil {
			return err
		}
	}

	indexDuration := time.Since(startTime)
	indexDurationSeconds := float64(indexDuration) / float64(time.Second)
	timePerDoc := float64(indexDuration) / float64(count)
	idx.l.Infof("Indexed %d documents, in %.2fs (average %.2fms/doc)", count, indexDurationSeconds, timePerDoc/float64(time.Millisecond))

	return nil
}
