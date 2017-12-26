package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// https://www.twilio.com/blog/2014/10/making-and-receiving-phone-calls-with-golang.html

func main() {
	var (
		addr        = flag.String("addr", ":6175", "listen address")
		authFile    = flag.String("auth", "", "file containing HTTP Basic Auth user:pass")
		bypassFile  = flag.String("bypass", "", "file containing secret bypass code (optional)")
		forwardFile = flag.String("forward", "", "file containing forwarding phone number")
		recordings  = flag.String("recordings", "", "path to save recordings")
	)
	flag.Parse()

	var user, pass string
	if *authFile != "" {
		buf, err := ioutil.ReadFile(*authFile)
		if err != nil {
			log.Fatal(err)
		}
		str := strings.TrimSpace(string(buf))
		toks := strings.SplitN(str, ":", 2)
		if len(toks) != 2 {
			log.Fatalf("%s: must be in format: user:pass", *authFile)
		}
		user, pass = toks[0], toks[1]
		if user == "" || pass == "" {
			log.Fatalf("%s: empty user or pass", *authFile)
		}
		log.Printf("%s: valid auth received", *authFile)
	} else {
		log.Fatal("no auth file specified, cannot start up")
	}

	var forward string
	if *forwardFile != "" {
		buf, err := ioutil.ReadFile(*forwardFile)
		if err != nil {
			log.Fatal(err)
		}
		forward = string(bytes.TrimSpace(buf))
		if len(forward) <= 0 {
			log.Fatalf("%s: forwarding number must be nonempty", *forwardFile)
		}
		if !isNumeric(forward) {
			log.Fatalf("%s: forwarding number must be numeric", *forwardFile)
		}
		log.Printf("%s: valid forwarding number received", *forwardFile)
	} else {
		log.Fatal("no forward file specified, cannot start up")
	}

	var bypass string
	if *bypassFile != "" {
		buf, err := ioutil.ReadFile(*bypassFile)
		if err != nil {
			log.Fatal(err)
		}
		bypass = string(bytes.TrimSpace(buf))
		if len(bypass) <= 0 {
			log.Fatalf("%s: bypass code must be nonempty", *bypassFile)
		}
		if !isNumeric(bypass) {
			log.Fatalf("%s: bypass code must be numeric", *bypassFile)
		}
		log.Printf("%s: valid bypass code received", *bypassFile)
	} else {
		log.Printf("no bypass file specified, bypass code not enabled")
	}

	if fi, err := os.Stat(*recordings); err != nil {
		log.Fatalf("recordings directory: %v", err)
	} else if !fi.IsDir() {
		log.Fatalf("recordings directory: %s isn't a directory", *recordings)
	}
	testfile := filepath.Join(*recordings, "make-sure-writes-work")
	if err := ioutil.WriteFile(testfile, []byte{}, 0600); err != nil {
		log.Fatalf("recordings directory: %v", err)
	}
	if err := os.Remove(testfile); err != nil {
		log.Fatalf("recordings directory: %v", err)
	}
	log.Printf("%s: saving recordings here", *recordings)

	http.Handle("/v1/greeting", logging(handleGreeting()))
	http.Handle("/v1/bypass", logging(handleBypass(bypass)))
	http.Handle("/v1/forward", logging(handleForward(forward)))
	http.Handle("/v1/recordings/", logging(handleRecordings(*recordings, "/v1/recordings", user, pass)))
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func handleVoice() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		xml.NewEncoder(w).Encode(struct {
			XMLName xml.Name `xml:"Response"`
			Say     string   `xml:",omitempty"`
		}{
			Say: fmt.Sprintf("Response %d. Generated %s.",
				atomic.LoadUint64(&globalRequestCounter),
				time.Now().UTC().Format("Monday 02 January, at 15, 04, 05"),
			),
		})
	})
}

func handleGreeting() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
			<Response> 
				<Gather input="dtmf" action="/v1/bypass" timeout="5" finishOnKey="#">
					<Say>Hello; enter code, or wait for connection.</Say>
				</Gather>
				<Redirect>/v1/forward</Redirect>
			</Response>
		`)
	})
}

func handleBypass(bypass string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		digits := r.FormValue("Digits")
		if digits == "" {
			digits = "empty"
		}
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
			<Response> 
				<Say>You entered the bypass code: %s. Thanks! Bye.</Say>
				<Hangup />
			</Response>
		`, digits)
	})
}

func handleForward(forward string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?> 
			<Response> 
				<Say>OK, connecting you now.</Say>
				<Dial record="record-from-ringing" recordingStatusCallback="/v1/recordings" recordingStatusCallbackMethod="POST">
					<Number>%s</Number>
				</Dial>
				<Say>Looks like there's no response. Sorry!</Say>
				<Hangup />
			</Response>
		`, forward)
	})
}

func handleRecordings(dir, prefix, user, pass string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		var (
			url = r.FormValue("RecordingUrl")
			sid = r.FormValue("RecordingSid")
			dur = r.FormValue("RecordingDuration")
		)
		if url == "" {
			handleListRecordings(user, pass, http.StripPrefix(prefix, fs)).ServeHTTP(w, r)
		} else {
			handlePostRecording(dir, url, sid, dur).ServeHTTP(w, r)
		}
	})
}

func handleListRecordings(user, pass string, fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requser, reqpass, _ := r.BasicAuth()
		if requser == user && reqpass == pass {
			fs.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="Field recordings"`)
		w.WriteHeader(401)
		fmt.Fprintln(w, "401 Unauthorized")
	})
}

func handlePostRecording(dir, url, sid, dur string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("POST Recording: %v", err)
			return
		}
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("POST Recording: %v", err)
			return
		}
		filename := time.Now().Format("2006-01-02-15-04-05") + "-" + dur + "sec" + "-" + sid + ".wav"
		if err := ioutil.WriteFile(filepath.Join(dir, filename), buf, 0600); err != nil {
			log.Printf("POST Recording: %v", err)
			return
		}
	})
}

var globalRequestCounter = uint64(0)

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			iw = &interceptingWriter{w, http.StatusOK}
			id = incr(&globalRequestCounter)
		)
		defer func(begin time.Time) {
			log.Printf("[%d] %s: %s %s (%dB)", id, r.RemoteAddr, r.Method, r.URL, r.ContentLength)
			for k, v := range r.Header {
				log.Printf("[%d] Header: %s: %s", id, k, v)
			}
			log.Printf("[%d] %d (%s)", id, iw.code, time.Since(begin))
		}(time.Now())

		next.ServeHTTP(iw, r)
	})
}

type interceptingWriter struct {
	http.ResponseWriter
	code int
}

func (iw *interceptingWriter) WriteHeader(code int) {
	iw.code = code
	iw.ResponseWriter.WriteHeader(code)
}

func incr(addr *uint64) uint64 {
	for {
		prev := atomic.LoadUint64(addr)
		next := prev + 1
		if atomic.CompareAndSwapUint64(addr, prev, next) {
			return next
		}
	}
}

func isNumeric(s string) bool {
	for _, r := range s {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			continue
		default:
			return false
		}
	}
	return true
}
