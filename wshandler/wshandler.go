package wshandler

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type device struct {
	ws        *websocket.Conn
	wrMu      sync.Mutex
	wsHandler *Devices
	devId     string
	done      chan bool
	send      chan []byte
}

type Devices struct {
	conn   map[string]*device
	mu     sync.RWMutex
	logger *log.Logger
	ctx    context.Context
}

type DeviceMess struct {
	MessageId string `json:"id"`
	DeviceId  string `json:"device_id"`
	Kind      int    `json:"kind"`
	Message   string `json:"message"`
}

const (
	StatusOK               = 200
	StatusDeviceNotFound   = 404
	StatusErrorWritingToWS = 500
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{} // use default options

func NewHandler(logger *log.Logger, ctx context.Context) Devices {
	return Devices{
		conn:   make(map[string]*device),
		logger: logger,
		ctx:    ctx,
	}
}

func (d *Devices) getConn(deviceId string) (*device, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	dev, ok := d.conn[deviceId]
	return dev, ok
}

func (d *Devices) saveConn(deviceId string, wsc *device) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conn[deviceId] = wsc
}

func (d *Devices) delConn(deviceId string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conn, deviceId)
}

func (d *Devices) iterateConn(perform func(string, *device)) {
	//safe concurrent range access to map
	//execute function "perform" for each map element, return slice of errors of each call
	d.mu.RLock()
	defer d.mu.RUnlock()
	for devId, dev := range d.conn {
		d.mu.RUnlock()
		perform(devId, dev)
		d.mu.RLock()
	}
}

func (d *Devices) WsEstablishDevConn(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		d.logger.Println("upgrade:", err)
		return
	}
	deviceId := r.URL.Query().Get("device_id")
	if deviceId == "" {
		clsMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "device_id is expected")
		wErr := c.WriteMessage(websocket.CloseMessage, clsMsg)
		if wErr != nil {
			d.logger.Println("write :", err)
		}
		d.logger.Println(r.RemoteAddr, "device id isn't provided")
		c.Close()
		return
	}

	dev := device{
		ws:        c,
		wsHandler: d,
		devId:     deviceId,
		done:      make(chan bool),
		send:      make(chan []byte, 256),
	}
	d.saveConn(deviceId, &dev)

	go func(dev *device) {
		dev.WriteWS()
	}(&dev)

	c.SetReadLimit(maxMessageSize)
	c.SetReadDeadline(time.Now().Add(pongWait))
	c.SetPongHandler(func(string) error { c.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		//For reading pong replies and close message. Message contents is not of any interest for us
		_, _, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseNormalClosure) {
				d.logger.Printf("error reading from websocket of device_id: %s. Error: %v", deviceId, err)
			}
			dev.done <- true
			d.delConn(deviceId)
			c.Close()
			break
		}
	}
}

func (d *Devices) SendMessage(devId, message, mesId, rAddr string) {

	if devId == "" {
		//broadcast
		d.iterateConn(func(devId string, dev *device) {
			//write to socket for each active connection
			dev.send <- []byte(message)
		})
	} else {
		//send to specific device_id
		dev, found := d.getConn(devId)
		if !found {
			format := "message id %s, from '%s', device_id '%s' not registered"
			d.logger.Printf(format, mesId, rAddr, devId)
			return
		}

		dev.send <- []byte(message)
	}

}

func (dev *device) WriteWS() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		dev.ws.Close()
		dev.wsHandler.delConn(dev.devId)
	}()

	for {
		select {
		case <-ticker.C:
			dev.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := dev.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				dev.wsHandler.logger.Printf("error pinging websocket of device id %s. Closing websocket", dev.devId)
				return
			}
		case message := <-dev.send:
			//!!!read whole buffer
			dev.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := dev.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				dev.wsHandler.logger.Printf("error writing to websocket of device id %s. Closing websocket", dev.devId)
				return
			}
		case <-dev.done:
			return
		case <-dev.wsHandler.ctx.Done():
			return
		}
	}
}
