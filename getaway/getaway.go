package getaway

import (
	"encoding/json"
	"html/template"
	"http-ws-getaway/wshandler"
	"log"
	"net/http"
)

type WSGateway struct {
	SrvAddr      string
	SendApiURI   string
	DevicesWsURI string
	Logger       *log.Logger
	HomeTemplate *template.Template
	wsHandler    wshandler.Devices
}

func (g *WSGateway) StartGetaway() {

	g.wsHandler = wshandler.NewHandler(g.Logger)
	router := http.NewServeMux()
	router.HandleFunc(g.SendApiURI, g.sendMessageToDevice)
	router.HandleFunc(g.DevicesWsURI, g.wsHandler.WsEstablishDevConn)
	router.HandleFunc("/", g.homeHandler)

	server := &http.Server{
		Addr:     g.SrvAddr,
		Handler:  router,
		ErrorLog: g.Logger,
	}

	g.Logger.Fatal(server.ListenAndServe())
}

func (g *WSGateway) sendMessageToDevice(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	m := wshandler.DeviceMess{}
	err := decoder.Decode(&m)
	if err != nil {
		g.Logger.Println("message decode from ", r.RemoteAddr, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - wrong message format"))
		return
	}

	sendStatus := g.wsHandler.SendMessage(m.DeviceId, m.Message, m.MessageId, r.RemoteAddr)
	if sendStatus.Code == wshandler.StatusOK {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sendStatus.Text))
	} else if sendStatus.Code == wshandler.StatusDeviceNotFound {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 - " + sendStatus.Text))
	} else if sendStatus.Code == wshandler.StatusErrorWritingToWS {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("505 - " + sendStatus.Text))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("505 - unknown status code. Status text: " + sendStatus.Text))
	}
}

func (g *WSGateway) homeHandler(w http.ResponseWriter, r *http.Request) {
	g.HomeTemplate.Execute(w, "ws://"+r.Host+"/api/device-ws")
}
