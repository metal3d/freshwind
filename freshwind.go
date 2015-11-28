package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/kwiscale/framework.v1"
)

var (
	VERSION          = "master"
	ROOT             = "."
	ADDR             = ":8000"
	FILTER           = ".*"
	TIME       int64 = 1000
	EXCLUDE          = `^\.`
	CONNS            = make(map[*websocket.Conn]bool)
	LIVERELOAD       = "js/__live__reload__script_"
	excludes         = make([]*regexp.Regexp, 0)
	filters          = make([]*regexp.Regexp, 0)
)

const JS = `(function(){
	var w;
	var connecting = false;
	function connect(){
		if (connecting) {
			return;
		}
		try {
			connecting = true;
			w = new WebSocket("ws://%s");

			w.onclose = function(){
				console.error("Connection closed, try to reconnect");
				connecting = false;
				setTimeout(function(){
					connect();
				}, 1000);
			};

			w.onopen = function(){
				console.info("Connected to reload websocket")
				connecting = false;
			};

			w.onmessage = function(m){
				var d = JSON.parse(m.data);
				if (d.reload) {
					document.location.reload();
				}
			};

		} catch(e) {
			connecting = false;
			w = null;
			setTimeout(connect, 1000);
		}
	}
	connect();
})();`

// JSReloadHandler serve the livereload script.
type JSReloadHandler struct{ kwiscale.RequestHandler }

// Get respond to the javascript request.
func (j *JSReloadHandler) Get() {
	jscode := fmt.Sprintf(JS, j.Request().Host+"/"+LIVERELOAD)
	j.Response().Header().Add("Content-Type", "application/javascript")
	j.WriteString(jscode)
}

// StaticHandler serves files.
type StaticHandler struct{ kwiscale.RequestHandler }

// Get serve files and inject livereload javascript if needed.
func (s *StaticHandler) Get() {
	p := s.Vars["path"]
	if p == "" {
		p = "index.html"
		if _, err := os.Stat(p); err != nil {
			p = "index.htm"
		}
	}
	content, err := ioutil.ReadFile(p)
	ext := filepath.Ext(p)
	if err != nil {
		log.Println(err)
		s.Status(404)
	} else {
		switch ext {
		case ".html", ".htm":
			cs := string(content)
			cs = strings.Replace(cs, "</body>", `<script src="/`+LIVERELOAD+`.js"></script>`+"\n</body>", 1)
			content = []byte(cs)
		}

		s.Response().Header().Add("Content-Type", mime.TypeByExtension(ext))

		s.Write(content)
	}
}

// WSHandler respond to the websocket that is injected in
// html files.
type WSHandler struct{ kwiscale.WebSocketHandler }

// Serve keep connections while some browser
// are connected.
func (w *WSHandler) Serve() {
	for {
		c := w.GetConnection()
		CONNS[c] = true
		c.ReadMessage()
	}
}

// waitAndReload listens for file changes and launches
// a message in websocket as soon as a file is changed.
func waitAndReload() {
	b, _ := filepath.Abs(ROOT)
	lastevt := time.Now().Unix()
	shouldreload := false
	for {

		filepath.Walk(b, func(p string, fi os.FileInfo, err error) error {

			// directories are not very interessing
			if fi.IsDir() {
				return nil
			}

			// filters file names to exclude
			for _, f := range excludes {
				if f.MatchString(fi.Name()) {
					return nil
				}
			}

			// now check if filename is in the filter list
			for _, f := range filters {
				if !f.MatchString(fi.Name()) {
					return nil
				}
			}

			// check modtime to be > of last sent event
			mt := fi.ModTime().Unix()
			if mt > lastevt {
				log.Println(p, "changed")
				lastevt = mt
				shouldreload = true
			}
			return nil
		})

		if shouldreload {
			for c, _ := range CONNS {
				if err := c.WriteJSON(map[string]bool{
					"reload": true,
				}); err != nil {
					log.Println(err, "so remove this connection...")
					delete(CONNS, c)
				}
			}
		}
		shouldreload = false
		time.Sleep(time.Duration(TIME) * time.Millisecond)
	}

}

func main() {

	flag.StringVar(&ROOT, "d", ROOT, "directory to serve")
	flag.StringVar(&ADDR, "a", ADDR, "address to serve")
	flag.StringVar(&FILTER, "f", FILTER, "coma separated list of regexp to match files")
	flag.StringVar(&EXCLUDE, "e", EXCLUDE, "coma separated list of regexp to exclude files")
	flag.Int64Var(&TIME, "t", TIME, "Time in milisecond for file change check")
	flag.StringVar(&LIVERELOAD, "s", LIVERELOAD, "script name that is used for websocker path and js script")
	v := flag.Bool("version", false, "show version")
	flag.Parse()

	if *v {
		fmt.Println(VERSION)
		return
	}

	f := strings.Split(FILTER, ",")
	for _, filter := range f {
		r := regexp.MustCompile(filter)
		filters = append(filters, r)
	}

	e := strings.Split(EXCLUDE, ",")
	for _, filter := range e {
		r := regexp.MustCompile(filter)
		excludes = append(excludes, r)
	}

	go waitAndReload()

	app := kwiscale.NewApp(&kwiscale.Config{
		Port:           ADDR,
		NbHandlerCache: 5,
	})

	app.AddRoute("/"+LIVERELOAD+".js", &JSReloadHandler{})
	app.AddRoute("/"+LIVERELOAD, &WSHandler{})
	app.AddRoute("/{path:.*}", &StaticHandler{})

	app.ListenAndServe()
}
