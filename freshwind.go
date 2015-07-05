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
	"gopkg.in/kwiscale/framework.v0"
)

var (
	VERSION          = "master"
	ROOT             = "."
	ADDR             = ":8000"
	FILTER           = `^\.`
	TIME       int64 = 1000
	_FILTERS         = make([]*regexp.Regexp, 0)
	CONNS            = make(map[*websocket.Conn]bool)
	LIVERELOAD       = "__live__reload__script_"
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

type JSReloadHandler struct{ kwiscale.RequestHandler }

func (j *JSReloadHandler) Get() {
	jscode := fmt.Sprintf(JS, j.Request.Host+"/"+LIVERELOAD)
	j.Response.Header().Add("Content-Type", "application/javascript")
	j.WriteString(jscode)
}

type StaticHandler struct{ kwiscale.RequestHandler }

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
			cs = strings.Replace(cs, "</body>", `<script src="`+LIVERELOAD+`.js"></script>`+"\n</body>", 1)
			content = []byte(cs)
		}

		s.Response.Header().Add("Content-Type", mime.TypeByExtension(ext))

		s.Write(content)
	}
}

type WSHandler struct{ kwiscale.WebSocketHandler }

func (w *WSHandler) Serve() {
	c := w.GetConnection()
	CONNS[c] = true
}

func waitAndReload() {
	b, _ := filepath.Abs(ROOT)
	lastevt := time.Now().Unix()
	shouldreload := false
	for {

		filepath.Walk(b, func(p string, fi os.FileInfo, err error) error {

			// filters file names to exclude
			for _, f := range _FILTERS {
				if f.MatchString(fi.Name()) {
					return nil
				}
			}
			// check modtime to be > of last sent event
			mt := fi.ModTime().Unix()
			if mt > lastevt {
				log.Println(fi.Name(), "changed")
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
	flag.StringVar(&FILTER, "f", FILTER, "coma separated list of regexp to exclude files")
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
		log.Println("filters", filter)
		r := regexp.MustCompile(filter)
		_FILTERS = append(_FILTERS, r)
	}

	go waitAndReload()

	app := kwiscale.NewApp(&kwiscale.Config{
		Port: ADDR,
	})

	app.AddRoute("/"+LIVERELOAD, WSHandler{})
	app.AddRoute("/"+LIVERELOAD+".js", JSReloadHandler{})
	app.AddRoute("/{path:.*}", StaticHandler{})

	app.ListenAndServe()
}
