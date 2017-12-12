package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
)

// https://www.twilio.com/blog/2014/10/making-and-receiving-phone-calls-with-golang.html

func main() {
	var (
		addr     = flag.String("addr", ":6175", "listen address")
		sentence = flag.String("sentence", "At MegaCorp, my voice is my password.", "sentence for /speak route")
	)
	flag.Parse()
	http.Handle("/v1/voice", handleSpeak(*sentence))
	http.Handle("/v1/message", handleMessage())
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type saying struct {
	XMLName xml.Name `xml:"Response"`
	Say     string   `xml:",omitempty"`
}

func handleSpeak(sentence string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		xml.NewEncoder(w).Encode(saying{Say: sentence})
	})
}

func handleMessage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})
}
