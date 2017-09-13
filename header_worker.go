package main

import (
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/sirkon/logcarrier/cheapbuf"
	"github.com/sirkon/logcarrier/connio"
	"github.com/sirkon/logcarrier/logging"
	"github.com/sirkon/logcarrier/utils"
)

// HeaderJob receives net.Conn item to route it further
type HeaderJob struct {
	Conn net.Conn
}

// HeaderPool parses headers and sent data to the appropriate worker
type HeaderPool struct {
	root          utils.PathGen
	headerjobs    chan HeaderJob
	dumpjobs      chan DumpJob
	logrotatejobs chan LogrotateJob

	rotname     func(string) string
	mkdir       func(string) error
	jobsCounter int
	wg          *sync.WaitGroup
	stopQueue   chan int
}

// NewHeaderPool constructor
func NewHeaderPool(
	headerjobs chan HeaderJob,
	dumpjobs chan DumpJob,
	logrotatejobs chan LogrotateJob,
) *HeaderPool {

	return &HeaderPool{
		headerjobs:    headerjobs,
		dumpjobs:      dumpjobs,
		logrotatejobs: logrotatejobs,

		jobsCounter: 0,
		wg:          &sync.WaitGroup{},
		stopQueue:   make(chan int),
	}
}

// Stop stop the job pool
func (hp *HeaderPool) Stop() {
	logging.Info("Stopping routing jobs")
	for i := 0; i < hp.jobsCounter; i++ {
		hp.stopQueue <- 0
	}
	hp.wg.Wait()
	logging.Info("Done")
}

// Spawn spawns a job
func (hp *HeaderPool) Spawn() {
	hp.jobsCounter++
	go func() {
		hp.wg.Add(1)
		wrk := &headerWorker{
			scanner: cheapbuf.NewScanner(cheapbuf.NewReaderSize(1024)),
			conn:    connio.NewReader(time.Second * 60),
			parser:  &headerParser{},
		}
		for {
			select {
			case x := <-hp.headerjobs:
				if err := wrk.parseHeader(x); err != nil {
					logging.Error("SERVER: %s", err)
					if err := x.Conn.Close(); err != nil {
						logging.Error("SERVER: error closing incoming connection %s", err)
					}
					continue
				}
				dirname := string(wrk.parser.Dirname)
				fname := string(wrk.parser.Logname)
				groupname := string(wrk.parser.Group)
				switch *(*string)(unsafe.Pointer(&wrk.parser.Command)) {
				case "DATA":
					hp.dumpjobs <- DumpJob{
						Dir:   dirname,
						Name:  fname,
						Group: groupname,
						Size:  int(wrk.parser.Size),
						Conn:  x.Conn,
					}
				case "ROTATE":
					hp.logrotatejobs <- LogrotateJob{
						Dir:   dirname,
						Name:  fname,
						Group: groupname,
						Conn:  x.Conn,
					}
				default:
					logging.Error("SERVER: error command %s", string(wrk.parser.Command))
					if err := x.Conn.Close(); err != nil {
						logging.Error("SERVER: error closing incoming connection %s", err)
					}
				}
			case <-hp.stopQueue:
				hp.wg.Done()
				return
			}
		}
	}()
}

type headerWorker struct {
	scanner *cheapbuf.Scanner
	conn    *connio.Reader
	parser  *headerParser
}

func (w *headerWorker) parseHeader(x HeaderJob) error {
	w.conn.SetConn(x.Conn)
	w.scanner.SetReader(w.conn)
	if !w.scanner.Scan() {
		return fmt.Errorf("got no response from %s", x.Conn.RemoteAddr().String())
	}
	if w.scanner.Err() != nil {
		return fmt.Errorf("failed to receive response from %s: %s", x.Conn.RemoteAddr().String(), w.scanner.Err())
	}
	if ok, err := w.parser.Parse(w.scanner.Bytes()); !ok || err != nil {
		e := fmt.Sprintf("failed to parse response `%s` from %s", string(w.scanner.Bytes()), x.Conn.RemoteAddr().String())
		if err != nil {
			e += fmt.Sprintf(": %s", err)
		}
		return fmt.Errorf(e)
	}
	return nil
}
