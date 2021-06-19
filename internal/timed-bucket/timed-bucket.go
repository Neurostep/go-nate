package timed_bucket

import (
	bookmarks_manager "github.com/Neurostep/go-nate/internal/bookmarks-manager"
	"time"
)

type (
	Bucket struct {
		List []bookmarks_manager.BookmarkToSave
	}
)

func NewBucket(list []bookmarks_manager.BookmarkToSave) *Bucket {
	return &Bucket{List: list}
}

func (b *Bucket) Add(item bookmarks_manager.BookmarkToSave) {
	b.List = append(b.List, item)
}

func (b *Bucket) GetEach(after time.Duration) chan bookmarks_manager.BookmarkToSave {
	ch := make(chan bookmarks_manager.BookmarkToSave)

	go func() {
		for _, item := range b.List {
			ch <- item
			time.Sleep(after)
		}
	}()

	return ch
}
