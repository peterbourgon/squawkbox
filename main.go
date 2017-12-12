package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// https://www.twilio.com/blog/2014/10/making-and-receiving-phone-calls-with-golang.html

func main() {
	var (
		addr     = flag.String("addr", ":6175", "listen address")
		sentence = flag.String("sentence", "At MegaCorp, my voice is my password.", "sentence for /speak route")
	)
	flag.Parse()
	http.Handle("/v1/voice", logging(handleSpeak(*sentence)))
	http.Handle("/v1/message", logging(handleMessage()))
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func handleSpeak(sentence string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		xml.NewEncoder(w).Encode(struct {
			XMLName xml.Name `xml:"Response"`
			Say     string   `xml:",omitempty"`
		}{
			Say: sentence,
		})
	})
}

func handleMessage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Not supported")
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
			log.Printf("[%d] %s: %s %s", id, r.RemoteAddr, r.Method, r.URL)
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
