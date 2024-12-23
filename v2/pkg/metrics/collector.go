package metrics

import (
	"context"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Collector interface {
	Update() error
	Name() string
}

type Runner struct {
	collectors []Collector
	interval   time.Duration
}

func NewRunner() *Runner {
	return &Runner{
		collectors: []Collector{},
		interval:   time.Second * 30,
	}
}

func (r *Runner) Register(collector Collector) {
	r.collectors = append(r.collectors, collector)
}

func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)

	for {
		<-ticker.C
		r.collect(ctx)
	}
}

func (r *Runner) collect(ctx context.Context) {
	logger := log.FromContext(ctx)
	wg := sync.WaitGroup{}
	wg.Add(len(r.collectors))
	for _, c := range r.collectors {
		go func(c Collector) {
			if err := c.Update(); err != nil {
				logger.Error(err, "failed to collect metrics", "name", c.Name())
			}
			wg.Done()
		}(c)
	}

	wg.Wait()
}
