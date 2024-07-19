package utils

import (
	"sync"
)

type Job func()

type WorkerPool struct {
	workerCount int
	jobQueue    chan Job
	stop        chan struct{}
	wg          sync.WaitGroup
}

func NewWorkerPool(workerCount int) *WorkerPool {
	return &WorkerPool{
		workerCount: workerCount,
		jobQueue:    make(chan Job, workerCount),
		stop:        make(chan struct{}),
	}
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go func() {
			defer wp.wg.Done()
			for {
				select {
				case job := <-wp.jobQueue:
					job()
				case <-wp.stop:
					return
				}
			}
		}()
	}
}

func (wp *WorkerPool) Submit(job Job) {
	select {
	case wp.jobQueue <- job:
	case <-wp.stop:
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.stop)
	wp.wg.Wait()
}
