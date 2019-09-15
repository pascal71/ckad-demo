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
	// "strconv" // Used to get integer from string
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
	message            = ""

	__IMAGE_NAME_VERSION__ = "TEMPLATE_IMAGE_NAME_VERSION"
	__IMAGE_VERSION__      = "TEMPLATE_IMAGE_VERSION"
	__DATE_VERSION__       = "TEMPLATE_DATE_VERSION"

        logo_base_path = "static/img/kubernetes_blue"
        logo_path     = "static/img/kubernetes_blue.txt"

	mux        = http.NewServeMux()
	listenAddr string = ":80"

	verbose    bool
	headers    bool

	livenessSecs int
	readinessSecs int

	dieafter int
	die  bool
	liveanddie bool
	readyanddie bool

	version  bool
)

type (
	Content struct {
		Title       string
		Hostname    string
		Hosttype    string
		Message     string
		PNG         string
		UsingImage  string
		NetworkInfo string
		FormattedReq   string
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
	/* We would normally parse command-line arguments like this
	   but we cannot pass arguments to flag.parse()
	   so we cannot use ENV vars as something to be parsed as cli args:
	flag.StringVar(&listenAddr, "listen", listenAddr, "listen address")
	*/
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
    formattedReq := formatRequest(r)

    fmt.Fprintf(w, "%s", formattedReq)
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

        //if (dieafter > 0) {
	//   if  now > started +dieafter {
            //log.Fatal("Dying once delay")
            //os.Exit(4)
	//   }
	//}

	if readyanddie {
            log.Fatal("Dying once ready")
            os.Exit(3)
	}

	hostName, err := os.Hostname()
	if err != nil {
		hostName = "unknown"
	}

        // Get user-agent: if text-browser, e.g. wget/curl/httpie/lynx/elinks return ascii-text image:
        //
        userAgent := r.Header.Get("User-Agent")

        multilineOP := (r.URL.Path != "/1line") && (r.URL.Path != "/1l") && (r.URL.Path != "/1")

        networkInfo := ""
        formattedReq   := ""
        if verbose && multilineOP {
            networkInfo = getNetworkInfo()
        }
        if headers {
            formattedReq   = formatRequest(r) + "\n"
        }

        // TOOD: bad
        // if CaseInsensitiveContains(__IMAGE_VERSION__, "bad") ||
        // }

	hostType := ""
	imageInfo := ""
	htmlPageTitle := ""
	msg := ""
        if message != "" { msg = "'" + message + "'" }

	if __IMAGE_NAME_VERSION__ == "" {
            hostType = "host"
	    htmlPageTitle = msg
	} else {
            hostType = "container"
	    imageInfo = "[image " + __IMAGE_NAME_VERSION__ + "]"
	    htmlPageTitle = msg + " " + imageInfo
	}

        if CaseInsensitiveContains(userAgent, "wget") ||
            CaseInsensitiveContains(userAgent, "curl") ||
            CaseInsensitiveContains(userAgent, "http") ||
            CaseInsensitiveContains(userAgent, "links") ||
            CaseInsensitiveContains(userAgent, "lynx") {
            w.Header().Set("Content-Type", "text/txt")

            logo_path  =  logo_base_path + "txt"

            fmt.Fprintf(w, "%s", formattedReq)
            var content []byte

	    if r.URL.Path == "/map" || r.URL.Path == "/MAP" {
	        content, _ = ioutil.ReadFile( MAP_ASCII_ART )
            } else {
	        content, _ = ioutil.ReadFile( logo_path )
            }

            myIP := GetOutboundIP()
            from := r.RemoteAddr
            fwd := r.Header.Get("X-Forwarded-For")

            d := " "
	    if multilineOP {
	        w.Write([]byte(content))
                d = "\n"
            }

            if fwd != "" { fwd=" [" + fwd + "]" }

	    p1 := fmt.Sprintf("Served from %s %s%s%s<%s>%s%s" + "Request from %s%s%s" + "%s%s" + "%s",
	        hostType, colour_me_yellow, hostName, imageInfo, myIP, colour_me_normal, d,
		from,fwd, d,
		networkInfo, d,
	        msg)

            fmt.Fprintf(w, p1 + "\n")

	    return
	}

        logo_path  =  logo_base_path + "png"

	// else return html as normal ...
        //
        templateFile := "templates/index.html.tmpl"

	template, err := loadTemplate(templateFile)
	if err != nil {
            log.Printf("error loading template from %s: %s\n", templateFile, err)
            return
	}

	cnt := &Content{
		Title:    htmlPageTitle,
		Hosttype: hostType,
		Hostname: hostName,
		Message:  message,
		PNG:      logo_path,
		UsingImage: imageInfo,
		NetworkInfo:  networkInfo,
		FormattedReq:  formattedReq,
        }

        // apply Context values to template
	template.Execute(w, cnt)
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

func showVersion(w http.ResponseWriter, r *http.Request) {
	resp := fmt.Sprintf("version: %s [%s]\n", __DATE_VERSION__, __IMAGE_NAME_VERSION__)
	w.Write([]byte(resp))
}

func setLogoPath() string {
    logo_base_path     = "static/img/kubernetes_"

    if CaseInsensitiveContains(__IMAGE_NAME_VERSION__, "docker-demo") {
        logo_base_path     = "static/img/docker_"
    }
    if CaseInsensitiveContains(__IMAGE_NAME_VERSION__, "k8s-demo") {
        logo_base_path     = "static/img/kubernetes_"
    }
    if CaseInsensitiveContains(__IMAGE_NAME_VERSION__, "ckad-demo") {
        logo_base_path     = "static/img/kubernetes_"
    }

    if CaseInsensitiveContains(__IMAGE_VERSION__, "1") {
        logo_base_path     +=  "blue."
    } else if CaseInsensitiveContains(__IMAGE_VERSION__, "2") {
        logo_base_path     +=  "red."
    } else if CaseInsensitiveContains(__IMAGE_VERSION__, "3") {
        logo_base_path     +=  "green."
    } else if CaseInsensitiveContains(__IMAGE_VERSION__, "4") {
        logo_base_path     +=  "cyan."
    } else if CaseInsensitiveContains(__IMAGE_VERSION__, "5") {
        logo_base_path     +=  "yellow."
    } else if CaseInsensitiveContains(__IMAGE_VERSION__, "6") {
        logo_base_path     +=  "white."
    } else {
        logo_base_path     +=  "blue."
    }

    logo_path = logo_base_path +  "txt" 

    return logo_path
}

// -----------------------------------
// func: main
//
//
//
func main() {
	f := flag.NewFlagSet("flag", flag.ExitOnError)

	f.StringVar(&listenAddr, "listen", listenAddr, "listen address")

	f.BoolVar(&die,         "die", false,   "die before live (false)")
	f.IntVar(&dieafter,     "dieafter", -1, "die after (NEVER)")

	f.BoolVar(&liveanddie,   "liveanddie",false,   "die once live (false)")
	f.IntVar(&livenessSecs,  "live",   0,   "liveness delay (0 sec)")
	f.IntVar(&livenessSecs,  "l",      0,   "liveness delay (0 sec)")

	f.BoolVar(&readyanddie,  "readyanddie",false,   "die once ready (false)")
	f.IntVar(&readinessSecs, "ready",  0,   "readiness delay (0 sec)")
	f.IntVar(&readinessSecs, "r",      0,   "readiness delay (0 sec)")

	f.StringVar(&__IMAGE_NAME_VERSION__, "image", __IMAGE_NAME_VERSION__, "image")
	f.StringVar(&__IMAGE_NAME_VERSION__, "i", __IMAGE_NAME_VERSION__, "image")

	f.StringVar(&message,            "message", "", "message")

	f.BoolVar(&version,      "version", false,  "Show version and exit")

	f.BoolVar(&verbose,      "verbose",false,   "verbose (false)")
	f.BoolVar(&verbose,      "v",      false,   "verbose (false)")
	f.BoolVar(&headers,      "headers",false,   "show headers (false)")
	f.BoolVar(&headers,      "h",      false,   "show headers (false)")

        // visitor := func(a *flag.Flag) { fmt.Println(">", a.Name, "value=", a.Value); }
        // fmt.Println("Visit()"); f.Visit(visitor) fmt.Println("VisitAll()")
        // f.VisitAll(visitor); fmt.Println("VisitAll()")

        if os.Getenv("CLI_ARGS") != "" {
            cli_args := os.Getenv("CLI_ARGS")
            a_cli_args := strings.Split(cli_args, " ")

            f.Parse(a_cli_args)
        } else {
            f.Parse(os.Args[1:])
        }
        // fmt.Println("Visit() after Parse()"); f.Visit(visitor);
        // fmt.Println("VisitAll() after Parse()") f.VisitAll(visitor)

        if verbose || version {
            log.Printf("%s Version: %s\n", os.Args[0], __DATE_VERSION__)

	    if version {
                os.Exit(0)
	    }

            log.Printf("%s\n", strings.Join(os.Args, " "))

        }

        if die {
            log.Fatal("Dying at beginning")
            os.Exit(1)
        }
        if (livenessSecs > 0) {
            //  Artificially sleep to simulate container initialization:
            delay := time.Duration(livenessSecs) * 1000 * time.Millisecond
            log.Printf("[liveness] Sleeping <%d> secs\n", livenessSecs)
            time.Sleep(delay)
        }
        if liveanddie {
            log.Fatal("Dying once live")
            os.Exit(2)
	}

	// ---- setup routes:

	// ---- act as static web server on /static/*
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	mux.HandleFunc("/", index)
	mux.HandleFunc("/test", statusCodeTest)

	mux.HandleFunc("/version", showVersion)

	mux.HandleFunc("/echo", formatRequestHandler)
	mux.HandleFunc("/ECHO", formatRequestHandler)

	mux.HandleFunc("/ping", ping)
	mux.HandleFunc("/PING", ping)

	mux.HandleFunc("/MAP", index)
	mux.HandleFunc("/map", index)

	if (readinessSecs > 0) {
            //  Artificially sleep to simulate application initialization:
            delay := time.Duration(readinessSecs) * 1000 * time.Millisecond
            log.Printf("[readiness] Sleeping <%d> secs\n", readinessSecs)
	    time.Sleep(delay)
        }
	if readyanddie {
            log.Fatal("Dying once ready")
            os.Exit(3)
	}

	logo_path = setLogoPath()
	if verbose {
            log.Printf("Default ascii art <%s>\n", logo_path)
        }

	log.Printf("Now listening on %s\n", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("error serving: %s", err)
	}

	// started=now
}
