package bookmarks_manager

import (
	"encoding/json"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"
	"io/ioutil"
	"os"
)

type (
	Manager struct {
		l    *zap.SugaredLogger
		b    *badger.DB
		from string
	}

	RootFolder struct {
		Folders []Bookmark `json:"folders"`
	}

	Bookmark struct {
		Type  string     `json:"type"`
		Href  string     `json:"href"`
		Title string     `json:"title"`
		Items []Bookmark `json:"items"`
	}

	BookmarkToSave struct {
		Title string `json:"title"`
		Path  string `json:"path"`
		Href  string `json:"href"`
	}

	BookmarksInfo struct {
		Total int
		Items []BookmarkToSave
	}

	BookmarkJson map[string]string

	Props struct {
		Logger   *zap.SugaredLogger
		FilePath string
		Db       *badger.DB
	}
)

const (
	_folderSeparator = "::"
)

func NewManager(props Props) (*Manager, error) {
	return &Manager{
		l:    props.Logger,
		b:    props.Db,
		from: props.FilePath,
	}, nil
}

func (m *Manager) ReadAll() (*BookmarksInfo, error) {
	var root RootFolder

	bmFile, err := os.Open(m.from)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := bmFile.Close()
		if err != nil {
			m.l.Error(err)
		}
	}()

	bytesValue, err := ioutil.ReadAll(bmFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(bytesValue, &root)
	if err != nil {
		return nil, err
	}

	links := flatten(root.Folders, "")

	return &BookmarksInfo{
		Total: len(links),
		Items: links,
	}, nil
}

func (m *Manager) Exists(href string) (bool, error) {
	var exists bool

	err := m.b.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(href))
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		exists = err != badger.ErrKeyNotFound

		return nil
	})

	return exists, err
}

func (m *Manager) Save(b BookmarkJson) error {
	return m.b.Update(func(txn *badger.Txn) error {
		bm, err := json.Marshal(&b)
		if err != nil {
			return err
		}

		return txn.Set([]byte(b["url"]), bm)
	})
}

func flatten(s []Bookmark, root string) (r []BookmarkToSave) {
	for _, b := range s {
		switch b.Type {
		case "link":
			bts := BookmarkToSave{
				Title: b.Title,
				Path:  root,
				Href:  b.Href,
			}
			r = append(r, bts)
		case "folder":
			newRoot := root
			if newRoot == "" {
				newRoot = b.Title
			} else {
				newRoot = fmt.Sprintf("%s%s%s", root, _folderSeparator, b.Title)
			}
			r = append(r, flatten(b.Items, newRoot)...)
		}
	}
	return
}
