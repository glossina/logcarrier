package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirkon/logcarrier/bufferer"
	"github.com/sirkon/logcarrier/fileio"
	"github.com/sirkon/logcarrier/frameio"
	"github.com/sirkon/logcarrier/logging"
	"github.com/sirkon/logcarrier/logio"
	"github.com/sirkon/logcarrier/paths"
	"github.com/sirkon/logcarrier/periodic"
	"github.com/sirkon/logcarrier/utils"

	"github.com/sirkon/logcarrier/notify"
	"github.com/sirkon/logcarrier/zstd"
	"gopkg.in/yaml.v2"
)

func getNotifier(data yaml.MapSlice) notify.Notifier {
	var cfg = struct {
		Type NotifierType `yaml:"type"`
	}{}
	b, err := yaml.Marshal(data)
	if err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		panic(err)
	}
	res, _ := GetNotifier(cfg.Type)
	if err := res.Init(b); err != nil {
		panic(err)
	}
	return res
}

func main() {
	cfgPath := flag.String("c", "/usr/local/etc/logcarrier.yaml", "configuration file path")
	flag.Parse()

	cfg := LoadConfig(*cfgPath)

	if len(cfg.LogFile) > 0 {
		loggingConfig := logging.NewConfig()
		loggingConfig.Logfile = cfg.LogFile
		if err := logging.SetConfig(loggingConfig); err != nil {
			panic(err)
		}
	}

	if !utils.PathExists(cfg.Files.Root) {
		fmt.Fprintf(os.Stderr, "Error: directory %s does not exist\n", cfg.Files.Root)
		os.Exit(1)
	}
	if len(cfg.Links.Root) > 0 {
		if !utils.PathExists(cfg.Links.Root) {
			fmt.Fprintf(os.Stderr, "Error: directory %s does not exist\n", cfg.Links.Root)
			os.Exit(1)
		}
	}

	fnamegens := paths.NewFiles(cfg.Files.Root, cfg.Files.Name, cfg.Files.Rotation)
	var lnamegens paths.Paths
	if cfg.Links.enabled {
		lnamegens = paths.NewLinks(cfg.Links.Root, cfg.Links.Name, cfg.Links.Rotation)
	} else {
		lnamegens = paths.Void(true)
	}

	// Setting up prerequisites
	headerjobs := make(chan HeaderJob, cfg.Buffers.Connections)
	dumpjobs := make(chan DumpJob, cfg.Buffers.Dumps)
	rotatejobs := make(chan LogrotateJob, cfg.Buffers.Logrotates)

	// Setting up notifiers
	fileBefore := getNotifier(cfg.Files.Before)
	fileAfter := getNotifier(cfg.Files.After)
	linkBefore := getNotifier(cfg.Links.Before)
	linkeAfter := getNotifier(cfg.Links.After)

	// factory creates bufferers what is needed to buffer incoming data
	var factory func(string, string, string) (bufferer.Bufferer, error)
	switch cfg.Compression.Method {
	case ZStd:
		factory = func(dir, name, group string) (bufferer.Bufferer, error) {
			d, err := fileio.Open(dir, name, group, fnamegens, lnamegens, cfg.Files.RootMode, fileBefore, linkBefore)
			if err != nil {
				return nil, err
			}
			f := frameio.NewWriterSize(d, int(cfg.Buffers.Framing))
			c := bufferer.NewZSTDWriter(func() *zstd.Writer {
				return zstd.NewWriterLevelDict(f, int(cfg.Compression.Level), make([]byte, cfg.Buffers.ZSTDict))
			})
			l := logio.NewWriterSize(c, int(cfg.Buffers.Input))
			return bufferer.NewZSTDBufferer(l, c, f, d), nil
		}
	case Raw:
		factory = func(dir, name, group string) (bufferer.Bufferer, error) {
			d, err := fileio.Open(dir, name, group, fnamegens, lnamegens, cfg.Files.RootMode, fileBefore, linkBefore)
			if err != nil {
				return nil, err
			}
			l := logio.NewWriterSize(d, int(cfg.Buffers.Input))
			return bufferer.NewRawBufferer(l, d), nil
		}
	}

	// Setting up background services
	ticker := time.NewTicker(time.Duration(cfg.Workers.FlusherSleep))

	fileNotifiers := NewNotifier(fileBefore, fileAfter)
	linkNotifiers := NewNotifier(linkBefore, linkeAfter)
	fileops := NewFileOp(factory, ticker, fileNotifiers, linkNotifiers)
	go fileops.FlushPeriodic()

	headerpool := NewHeaderPool(headerjobs, dumpjobs, rotatejobs)
	dumppool := NewDumpPool(dumpjobs, fileops, time.Duration(cfg.WaitTimeout))

	rotatepool := NewLogrotatePool(rotatejobs, fileops, time.Duration(cfg.WaitTimeout))
	switch cfg.Logrotate.Method {
	case LogrotatePeriodic:
		rotatepool.MakePlumb()
		go fileops.LogrotatePeriodic(periodic.Schedule(cfg.Logrotate.Schedule))
	case LogrotateBoth:
		go fileops.LogrotatePeriodic(periodic.Schedule(cfg.Logrotate.Schedule))
	}

	for i := 0; i < cfg.Workers.Router; i++ {
		headerpool.Spawn()
	}
	for i := 0; i < cfg.Workers.Dumper; i++ {
		dumppool.Spawn()
	}
	for i := 0; i < cfg.Workers.Logrotater; i++ {
		rotatepool.Spawn()
	}

	// Debug service
	if len(cfg.ListenDebug) > 0 {
		logging.Info("Debug listening on " + cfg.ListenDebug)
		go func() {
			if err := http.ListenAndServe(cfg.ListenDebug, nil); err != nil {
				panic(err)
			}
		}()
	}

	// Start serving
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	l, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		logging.Critical("Error listening: %s", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			logging.Error("Error closing listening socket: %s", err)
		}
	}()
	logging.Info("Listening on %s", cfg.Listen)
	acceptConn := true

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				logging.Critical("Error accepting: %s", err)
			}
			headerjobs <- HeaderJob{Conn: conn}
			if !acceptConn {
				break
			}
		}
	}()

sigloop:
	for {
		sig := <-signals
		switch sig {
		case os.Interrupt:
			logging.Info("SIGINT received")
			acceptConn = false
			break sigloop
		case syscall.SIGTERM:
			logging.Info("SIGTERM received")
			acceptConn = false
			break sigloop
		}
	}

	// Stopping services
	ticker.Stop()
	fileops.Join()
	headerpool.Stop()
	dumppool.Stop()
	rotatepool.Stop()
}
