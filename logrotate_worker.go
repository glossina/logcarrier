package main

import (
	"net"
	"sync"
	"time"

	"github.com/sirkon/logcarrier/logging"
)

var errorLogRotate = []byte("400 Error\n")
var doneLogRotate = []byte("200 DONE\n")

// LogrotateJob shows what file to rotate
type LogrotateJob struct {
	Dir   string
	Name  string
	Group string
	Conn  net.Conn
}

// LogrotatePool spawn workers what rotates log
type LogrotatePool struct {
	netjobs     chan LogrotateJob
	files       *FileOp
	jobsCounter int
	wg          *sync.WaitGroup
	stopQueue   chan int
	method      func(string, string, string) error
}

// NewLogrotatePool constructor
func NewLogrotatePool(netjobs chan LogrotateJob, files *FileOp, timeout time.Duration) *LogrotatePool {
	res := &LogrotatePool{
		netjobs:     netjobs,
		files:       files,
		jobsCounter: 0,
		wg:          &sync.WaitGroup{},
		stopQueue:   make(chan int),
	}
	res.method = res.LogrotateReal
	return res
}

// MakePlumb replaces logrotating with doing nothing
func (lr *LogrotatePool) MakePlumb() {
	lr.method = lr.LogrotatePlumb
}

// Stop command jobs to stop
func (lr *LogrotatePool) Stop() {
	logging.Info("Stopping log rotating jobs")
	for i := 0; i < lr.jobsCounter; i++ {
		lr.stopQueue <- 0
	}
	lr.wg.Wait()
	logging.Info("Done")
}

// Spawn spawns a worker
func (lr *LogrotatePool) Spawn() {
	lr.jobsCounter++
	go func() {
		lr.wg.Add(1)
		for {
			select {
			case x := <-lr.netjobs:
				err := lr.files.Logrotate(x.Dir, x.Name, x.Group)
				if err != nil {
					logging.Error("LOGROTATER: %s", err)
					if _, err := x.Conn.Write(errorLogRotate); err != nil {
						logging.Error("LOGROTATER: %s", err)
					}
				} else {
					logging.Error("LOGROTATER: rotating %s", x.Name)
					if _, err := x.Conn.Write(doneLogRotate); err != nil {
						logging.Error("LOGROTATER: %s", err)
					}
				}
				if err := x.Conn.Close(); err != nil {
					logging.Error("LOGROTATER: cannot close connection to %s: %s", x.Conn.RemoteAddr().String(), err)
				}
			case <-lr.stopQueue:
				lr.wg.Done()
				return
			}
		}
	}()
}

// LogrotateReal performs actual log rotating job for the given file path
func (lr *LogrotatePool) LogrotateReal(dir, name, group string) error {
	return lr.files.Logrotate(dir, name, group)
}

// LogrotatePlumb does nothing
func (lr *LogrotatePool) LogrotatePlumb(dir, name, group string) error {
	return nil
}
