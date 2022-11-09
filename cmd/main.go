package main

import (
	"flag"
	"html/template"
	"http-ws-getaway/getaway"
	"log"
	"os"
)

var addr = flag.String("addr", "localhost:8080", "http service address")
var debug = flag.Bool("debug", false, "Turn debug on")

func traceStart(logger *log.Logger) {
	logger.Println("trace started")
}

func traceStop(logger *log.Logger) {
	logger.Println("trace stopped")
}

func main() {
	flag.Parse()
	logger := log.New(os.Stdout, "http-ws-gateway: ", log.LstdFlags)
	gtw := getaway.WSGateway{
		SrvAddr:      *addr,
		SendApiURI:   "/api/send-message",
		DevicesWsURI: "/api/device-ws",
		Logger:       logger,
		HomeTemplate: homeTemplate,
	}
	if *debug {
		gtw.OnStart = func() { traceStart(logger) }
		gtw.OnStop = func() { traceStop(logger) }
	}
	gtw.StartGetaway()
}

var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<script>  
window.addEventListener("load", function(evt) {

    var output = document.getElementById("output");
    var input = document.getElementById("input");
    var ws;

    var print = function(message) {
        var d = document.createElement("div");
        d.textContent = message;
        output.appendChild(d);
        output.scroll(0, output.scrollHeight);
    };

    document.getElementById("open").onclick = function(evt) {
        if (ws) {
            return false;
        }
        ws = new WebSocket("{{.}}" + "?device_id=" + input.value);
        ws.onopen = function(evt) {
            print("OPEN");
        }
        ws.onclose = function(evt) {
            print("CLOSE " + evt.reason);
            ws = null;
        }
        ws.onmessage = function(evt) {
			print("RESPONSE: " + evt.data);
        }
        ws.onerror = function(evt) {
            print("ERROR: " + evt.data);
        }
        return false;
    };

    document.getElementById("close").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        ws.close();
        return false;
    };

});
</script>
</head>
<body>
<table>
<tr><td valign="top" width="50%">
<p>Click "Open" to create a connection to the server with device_id from input field.</p>
<p>"Close" to close the connection.</p>
<form>
<button id="open">Open</button>
<button id="close">Close</button>
<p><input id="input" type="text" value="00000000-0000-1111-2222-334455667788"></p>
</form>
</td><td valign="top" width="50%">
<div id="output" style="max-height: 70vh;overflow-y: scroll;"></div>
</td></tr></table>
</body>
</html>
`))
