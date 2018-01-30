package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/oklog/run"
)

func main() {
	fs := flag.NewFlagSet("squawkbox", flag.ExitOnError)
	var (
		addr          = fs.String("addr", "127.0.0.1:9176", "listen address")
		debug         = fs.Bool("debug", false, "debug logging")
		authfile      = fs.String("authfile", "", "file containing HTTP BasicAuth user:pass:realm")
		forwardfile   = fs.String("forwardfile", "", "file containing number to forward to")
		forward       = fs.String("forward", "Connecting you now.", "forward text")
		noResponse    = fs.String("noresponse", "Nobody picked up. Goodbye!", "no response text")
		eventsfile    = fs.String("eventsfile", "events.dat", "file to store event log")
		recordingsdir = fs.String("recordingsdir", "", "directory containing saved recordings")
	)
	fs.Usage = usageFor(fs, "squawkbox [flags]")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	var loglevel level.Option
	{
		loglevel = level.AllowInfo()
		if *debug {
			loglevel = level.AllowDebug()
		}
	}

	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stdout)
		logger = log.With(logger, "ts", log.DefaultTimestampUTC())
		logger = log.With(logger, "caller", log.Caller(5))
		logger = level.NewFilter(logger, loglevel)
	}

	var auditLog *auditLog
	{
		var err error
		auditLog, err = newAuditLog(*eventsfile)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
	}

	var basicAuthRealm, basicAuthUser, basicAuthPass string
	{
		var err error
		basicAuthRealm, basicAuthUser, basicAuthPass, err = parseAuthFile(*authfile)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
	}

	var forwardNumber string
	{
		var err error
		forwardNumber, err = parseForwardFile(*forwardfile)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
	}
	var recordingManager *recordingManager
	{
		recordingManager = newRecordingManager(*recordingsdir)
	}

	var handler http.Handler
	{
		router := mux.NewRouter()
		router.StrictSlash(true)
		registerAdminRoutes(router, basicAuthRealm, basicAuthUser, basicAuthPass, auditLog, recordingManager)
		registerDoorbellRoutes(router, *forward, forwardNumber, *noResponse, recordingManager)

		handler = router
		handler = auditingMiddleware(auditLog)(handler)
		handler = loggingMiddleware(logger)(handler)
	}

	var ln net.Listener
	{
		var err error
		ln, err = net.Listen("tcp", *addr)
		if err != nil {
			level.Error(logger).Log("module", "main", "err", err)
			os.Exit(1)
		}
	}

	var g run.Group
	{
		server := http.Server{Handler: handler}
		g.Add(func() error {
			level.Info(logger).Log("addr", *addr)
			return server.Serve(ln)
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			server.Shutdown(ctx)
		})
	}
	level.Info(logger).Log("exit", g.Run())
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			if f.DefValue == "" {
				f.DefValue = `...`
			}
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}
