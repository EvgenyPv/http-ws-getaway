package getaway

import (
	"context"
	"encoding/json"
	"html/template"
	"http-ws-getaway/wshandler"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type WSGateway struct {
	SrvAddr      string
	SendApiURI   string
	DevicesWsURI string
	Logger       *log.Logger
	HomeTemplate *template.Template
	OnStart      func()
	OnStop       func()
	wsHandler    wshandler.Devices
	ctx          context.Context
}

func (g *WSGateway) StartGetaway() {

	if g.OnStart != nil {
		g.OnStart()
	}
	g.ctx, _ = signal.NotifyContext(context.Background(), os.Interrupt)
	g.wsHandler = wshandler.NewHandler(g.Logger, g.ctx)
	router := gin.Default()
	router.POST(g.SendApiURI, g.sendMessageToDevice)
	router.GET(g.DevicesWsURI+"/:deviceId", g.wsHandler.WsEstablishDevConn)
	//due to bug in go tool trace? trace with SigInt doesn't work
	//router.GET("/stoptrace", func(c *gin.Context) { g.OnStop(); c.String(http.StatusOK, "Ok") })
	router.GET("/", g.homeHandler)

	server := &http.Server{
		Addr:     g.SrvAddr,
		Handler:  router,
		ErrorLog: g.Logger,
	}

	erG, ergCtx := errgroup.WithContext(g.ctx)
	erG.Go(func() error {
		return server.ListenAndServe()
	})

	erG.Go(func() error {
		<-ergCtx.Done()
		if g.OnStop != nil {
			g.OnStop()
		}
		return server.Shutdown(context.Background())
	})

	if err := erG.Wait(); err != nil {
		g.Logger.Printf("server exit reason: %s", err)
	}

}

func (g *WSGateway) sendMessageToDevice(c *gin.Context) {
	decoder := json.NewDecoder(c.Request.Body)
	m := wshandler.DeviceMess{}
	err := decoder.Decode(&m)
	if err != nil {
		g.Logger.Println("message decode from ", c.Request.RemoteAddr, err)
		c.String(http.StatusInternalServerError, "500 - wrong message format")
		return
	}

	g.wsHandler.SendMessage(m.DeviceId, m.Message, m.MessageId, c.Request.RemoteAddr)

	c.String(http.StatusOK, "Message is sent")
	/*if sendStatus.Code == wshandler.StatusOK {
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
	}*/
}

func (g *WSGateway) homeHandler(c *gin.Context) {
	g.HomeTemplate.Execute(c.Writer, "ws://"+c.Request.Host+"/api/device-ws")

}
