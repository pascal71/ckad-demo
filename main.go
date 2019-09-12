package main

// TODO:
//   Cleanup image info (spaces to separate from "Served from" message
//   Add verbose template index.html.tmpl.verbose.tmpl to handle extra info

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"io/ioutil"
	"time"    // Used for Sleep
	"strconv" // Used to get integer from string
	// "encoding/json"
)

const (
	// Courtesy of "telnet mapascii.me"
	MAP_ASCII_ART      = "static/img/mapascii.txt"

	escape             = "\x1b"
	colour_me_black    = escape + "[0;30m"
	colour_me_red      = escape + "[0;31m"
	colour_me_green    = escape + "[0;32m"
	colour_me_blue     = escape + "[0;34m"
	colour_me_yellow   = escape + "[1;33m"
	colour_me_normal   = escape + "[0;0m"
)

var (
	// -- defaults: can be overridden by cli ------------
	// -- defaults: to be overridden by env/cli/cm ------
	image_name_version = "mjbright/ckad-demo:1"
	image_version      = "1"

        logo_ascii_art     = "static/img/kubernetes_white.txt"

	mux        = http.NewServeMux()
	listenAddr string = ":80"

	verbose    bool
	headers    bool

	livenessSecs int
	readinessSecs int
)

type (
	Content struct {
		Title       string
		Hostname    string
		Image       string
		NetworkInfo string
		RequestPP   string
	}
)

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
    conn, err := net.Dial("udp", "8.8.8.8:80")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    localAddr := conn.LocalAddr().(*net.UDPAddr)

    return localAddr.IP
}

// -----------------------------------
// func: init
//
// Read command-line arguments:
//
func init() {
	flag.StringVar(&listenAddr, "listen", listenAddr, "listen address")

	flag.IntVar(&livenessSecs,  "l",      0,   "liveness delay (0 sec)")
	flag.IntVar(&readinessSecs, "r",      0,   "readiness delay (0 sec)")

	flag.StringVar(&image_name_version, "i", image_name_version, "image")

	flag.BoolVar(&verbose,      "v",      false,   "verbose (false)")
	flag.BoolVar(&headers,      "h",      false,   "show headers (false)")
}

// -----------------------------------
// func: loadTemplate
//
// load template file and substitute variables
//
func loadTemplate(filename string) (*template.Template, error) {
	return template.ParseFiles(filename)
}

// -----------------------------------
// func: CaseInsensitiveContains
//
// Do case insensitive match - of substr in s
//
func CaseInsensitiveContains(s, substr string) bool {
        s, substr = strings.ToUpper(s), strings.ToUpper(substr)
        return strings.Contains(s, substr)
}

// -----------------------------------
// func: formatRequestHandler
//
// generates ascii representation of a request
//
// From: https://medium.com/doing-things-right/pretty-printing-http-requests-in-golang-a918d5aaa000
//
func formatRequestHandler(w http.ResponseWriter, r *http.Request) {
    ret := formatRequest(r)

    fmt.Fprintf(w, "%s", ret)
    return
}

// -----------------------------------
// func: formatRequest
//
// generates ascii representation of a request
//
// From: https://medium.com/doing-things-right/pretty-printing-http-requests-in-golang-a918d5aaa000
//
func formatRequest(r *http.Request) string {
    // Create return string
    var request []string

    // Add the request string
    url := fmt.Sprintf("%v %v %v", r.Method, r.URL, r.Proto)
    request = append(request, url)

    // Add the host
    request = append(request, fmt.Sprintf("Host: %v", r.Host))

    // Loop through headers
    for name, headers := range r.Header {
        name = strings.ToLower(name)
        for _, h := range headers {
            request = append(request, fmt.Sprintf("%v: %v", name, h))
        }
    }

    // If this is a POST, add post data
    if r.Method == "POST" {
        r.ParseForm()
        request = append(request, "\n")
        request = append(request, r.Form.Encode())
    }

    // Return the request as a string
    return strings.Join(request, "\n")
}

// -----------------------------------
// func: statusCodeTest
//
// Example handler - sets status code
//
func statusCodeTest(w http.ResponseWriter, req *http.Request) {
    //m := map[string]string{
        //"foo": "bar",
    //}
    //w.Header().Add("Content-Type", "application/json")

    //num := http.StatusCreated
    num := http.StatusInternalServerError

    w.WriteHeader( num )

    //_ = json.NewEncoder(w).Encode(m)
    fmt.Fprintf(w, "\nWriting status code <%d>\n", num)
}

// -----------------------------------
// func: index
//
// Main index handler - handles different requests
//
func index(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request from '%s' (%s)\n", r.Header.Get("X-Forwarded-For"), r.URL.Path)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

        // image_version "3": return early with status code 404 or /CODE (from r.URL.Path)
        //
        if "image_version" == "3" {
            //fmt.Fprintf(w, "\nPATH=<%s>", r.URL.Path)
            //if r.URL.Path == "/map" || r.URL.Path == "/MAP" {}

            num, err := strconv.Atoi( r.URL.Path[1:] )
            if err == nil {
                // how to set numeric status code?
                // http.Error(w, http.StatusText(num), num)

                //num = http.StatusInternalServerError
                w.WriteHeader( num )
                fmt.Fprintf(w, "\nWriting status code <%d>\n", num)
            } else {
                fmt.Fprintf(w, "\nWriting default status code <NotFound>\n")
                http.NotFound(w, r)
            }

            return
        }

        // Get user-agent: if text-browser, e.g. wget/curl/httpie/lynx/elinks return ascii-text image:
        //
        userAgent := r.Header.Get("User-Agent")

        networkInfo := ""
        requestPP   := ""
        if verbose {
            networkInfo = getNetworkInfo()
        }
        if headers {
            requestPP   = formatRequest(r)
        }

        if CaseInsensitiveContains(userAgent, "wget") ||
            CaseInsensitiveContains(userAgent, "curl") ||
            CaseInsensitiveContains(userAgent, "httpie") ||
            CaseInsensitiveContains(userAgent, "links") ||
            CaseInsensitiveContains(userAgent, "lynx") {
            w.Header().Set("Content-Type", "text/txt")

            fmt.Fprintf(w, "\n%s", requestPP)
            var content []byte

	    if r.URL.Path == "/map" || r.URL.Path == "/MAP" {
	        content, _ = ioutil.ReadFile( MAP_ASCII_ART )
            } else {
	        content, _ = ioutil.ReadFile( logo_ascii_art )
            }

            myIP := GetOutboundIP()
            from := r.RemoteAddr
            fwd := r.Header.Get("X-Forwarded-For")

	    if r.URL.Path != "/1line" {
	        w.Write([]byte(content))
            }

            p1 := fmt.Sprintf("Served from container %s%s[%s]%s", colour_me_yellow, hostname, myIP, colour_me_normal)
            p2 := ""
            if fwd != "" {
                p2 = fmt.Sprintf("Request from %s [%s]", from, fwd)
            } else {
                p2 = fmt.Sprintf("Request from %s", from)
            }
            p3 := ""
            if networkInfo != "" {
                p3 = fmt.Sprintf("%s ", networkInfo)
	        if r.URL.Path != "/1line" {
                    p3 = p3 + "\n"
                }
            }
            p4 := fmt.Sprintf("image <%s>\n", image_name_version)

	    if r.URL.Path == "/1line" {
                fmt.Fprintf(w, p1 + " " + p2 + " " + p3 + p4)
            } else {
                fmt.Fprintf(w, "\n")
                fmt.Fprintf(w, p1)
                fmt.Fprintf(w, "\n")
                fmt.Fprintf(w, p2)
                fmt.Fprintf(w, "\n")
                fmt.Fprintf(w, p3)
                fmt.Fprintf(w, p4)
                fmt.Fprintf(w, "\n")
            }

	    return
	}

	// else return html as normal ...
        //
        templateFile := "templates/index.html.tmpl"

	t, err := loadTemplate(templateFile)
	if err != nil {
		log.Printf("error loading template from %s: %s\n", templateFile, err)
		return
	}

	title := os.Getenv("TITLE")

	cnt := &Content{
		Title:    title,
		Hostname: hostname,
		Image:    image_name_version,
		NetworkInfo:  networkInfo,
		RequestPP:  requestPP,
        }

        // apply Context values to template
	t.Execute(w, cnt)
}

func getNetworkInfo() string {
    ret := "Network interfaces:\n"

    ifaces, _ := net.Interfaces()
    // handle err
    for _, i := range ifaces {
        addrs, _ := i.Addrs()
        // handle err
        for _, addr := range addrs {
            var ip net.IP
            switch v := addr.(type) {
                case *net.IPNet:
                    ip = v.IP
                case *net.IPAddr:
                    ip = v.IP
                }
            // process IP address
            ret = fmt.Sprintf("%sip %s\n", ret, ip.String())
        }
    }
    ret = fmt.Sprintf("%slistening on port %s\n", ret, listenAddr)

    return ret
}

// -----------------------------------
// func: ping
//
// Ping handler - echos back remote address
//
func ping(w http.ResponseWriter, r *http.Request) {
	resp := fmt.Sprintf("ping: hello %s\n", r.RemoteAddr)
	w.Write([]byte(resp))
}

// -----------------------------------
// func: main
//
//
//
func main() {
	flag.Parse()

	//  Extract image_version from image_name_version (affects behaviour):
	if (strings.Contains(image_name_version, ":")) {
	    image_version=image_name_version[ 1+strings.Index(image_name_version, ":") : ]
            //log.Printf("\nExtracted image version <%s>\n", image_version)
	}

	if (livenessSecs > 0) {
            delay := time.Duration(livenessSecs) * 1000 * time.Millisecond
            log.Printf("\n[liveness] Sleeping <%d> secs\n", livenessSecs)
	    time.Sleep(delay)
        }

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	mux.HandleFunc("/", index)
	mux.HandleFunc("/test", statusCodeTest)

	mux.HandleFunc("/echo", formatRequestHandler)
	mux.HandleFunc("/ECHO", formatRequestHandler)

	mux.HandleFunc("/ping", ping)
	mux.HandleFunc("/PING", ping)

	mux.HandleFunc("/MAP", index)
	mux.HandleFunc("/map", index)

	if (readinessSecs > 0) {
            delay := time.Duration(readinessSecs) * 1000 * time.Millisecond
            log.Printf("\n[readiness] Sleeping <%d> secs\n", readinessSecs)
	    time.Sleep(delay)
        }

	log.Printf("listening on %s\n", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("error serving: %s", err)
	}
}
