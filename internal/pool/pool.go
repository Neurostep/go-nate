package pool

import (
	"fmt"
	"time"
)

var ErrScheduleTimeout = fmt.Errorf("schedule error: timed out")

type Pool struct {
	sem chan struct{}
}

func NewPool(size int) *Pool {
	p := &Pool{
		sem: make(chan struct{}, size),
	}

	return p
}

func (p *Pool) Schedule(task func()) {
	_ = p.schedule(task, nil)
}

func (p *Pool) ScheduleTimeout(timeout time.Duration, task func()) error {
	return p.schedule(task, time.After(timeout))
}

func (p *Pool) schedule(task func(), timeout <-chan time.Time) error {
	select {
	case <-timeout:
		return ErrScheduleTimeout
	case p.sem <- struct{}{}:
		go p.worker(task)
		return nil
	}
}

func (p *Pool) worker(task func()) {
	defer func() { <-p.sem }()

	task()
}
