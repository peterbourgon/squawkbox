package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
)

func main() {
	fs := flag.NewFlagSet("squawkbox", flag.ExitOnError)
	var (
		addr          = fs.String("addr", "127.0.0.1:9176", "listen address")
		debug         = fs.Bool("debug", false, "debug logging")
		authfile      = fs.String("authfile", "", "file containing HTTP BasicAuth user:pass:realm")
		forwardfile   = fs.String("forwardfile", "", "file containing number to forward to")
		greeting      = fs.String("greeting", "Hello; enter code, or wait for connection.", "greeting text")
		forward       = fs.String("forward", "Connecting you now.", "forward text")
		noResponse    = fs.String("noresponse", "Nobody picked up. Goodbye!", "no response text")
		codesfile     = fs.String("codesfile", "codes.dat", "file to store bypass codes")
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

	var eventLog *eventLog
	{
		var err error
		entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
		eventLog, err = newEventLog(*eventsfile, entropy)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
	}

	var authenticate func(http.Handler) http.Handler
	{
		if *authfile != "" {
			realm, user, pass, err := parseAuthFile(*authfile)
			if err != nil {
				level.Error(logger).Log("err", err)
				os.Exit(1)
			}
			authenticate = func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requser, reqpass, _ := r.BasicAuth()
					if requser != user || reqpass != pass {
						w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
						w.WriteHeader(http.StatusUnauthorized)
						fmt.Fprintln(w, http.StatusText(http.StatusUnauthorized))
						return
					}
					next.ServeHTTP(w, r)
				})
			}
			level.Debug(logger).Log("basic_auth", "enabled")
		} else {
			authenticate = func(next http.Handler) http.Handler { return next }
			level.Warn(logger).Log("basic_auth", "disabled")
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

	var codeManager *codeManager
	{
		var err error
		codeManager, err = newCodeManager(*codesfile)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
	}

	var recordingManager *recordingManager
	{
		recordingManager = newRecordingManager(*recordingsdir)
	}

	api := &api{
		EventLog:         eventLog,
		Authenticate:     authenticate,
		GreetingText:     *greeting,
		ForwardText:      *forward,
		ForwardNumber:    forwardNumber,
		NoResponseText:   *noResponse,
		CodeManager:      codeManager,
		RecordingManager: recordingManager,
	}

	server := http.Server{
		Handler: api,
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		level.Error(logger).Log("module", "main", "err", err)
		os.Exit(1)
	}

	var g run.Group
	{
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
