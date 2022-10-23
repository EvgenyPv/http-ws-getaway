package wshandler

import (
	"fmt"
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
}

type Devices struct {
	conn           map[string]*device
	mu             sync.RWMutex
	logger         *log.Logger
	maxMessageSize int64
}

type DeviceSendStatus struct {
	Code int
	Text string
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

func NewHandler(logger *log.Logger) Devices {
	return Devices{
		conn:   make(map[string]*device),
		logger: logger,
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

func (d *Devices) iterateConn(perform func(string, *device) error) []error {
	//safe concurrent range access to map
	//execute function "perform" for each map element, return slice of errors of each call
	var res []error
	res = make([]error, len(d.conn))
	d.mu.RLock()
	defer d.mu.RUnlock()
	i := 0
	for devId, dev := range d.conn {
		d.mu.RUnlock()
		res[i] = perform(devId, dev)
		i++
		d.mu.RLock()
	}

	return res
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

	dev := device{ws: c, wsHandler: d, devId: deviceId, done: make(chan bool)}
	d.saveConn(deviceId, &dev)

	go func(dev *device) {
		dev.pingWS()
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

func (d *Devices) SendMessage(devId, message, mesId, rAddr string) DeviceSendStatus {

	result := DeviceSendStatus{}

	if devId == "" {
		//broadcast
		errs := d.iterateConn(func(devId string, dev *device) error {
			//write to socket for each active connection
			return dev.writeWS(websocket.TextMessage, []byte(message))
		})

		if len(errs) == 0 {
			statTxt := fmt.Sprintf("message id %s, from '%s', no registered devices", mesId, rAddr)
			d.logger.Printf(statTxt)
			result.Code = StatusDeviceNotFound
			result.Text = statTxt
			return result
		}
		errCnt := 0
		okCnt := 0
		iLastErr := 0
		for i, err := range errs {
			if err == nil {
				okCnt++
			} else {
				errCnt++
				iLastErr = i
			}
		}

		if okCnt == 0 {
			format := "message id %s, from '%s', error broadcasting to devices. Last err: %s"
			statTxt := fmt.Sprintf(format, mesId, rAddr, errs[iLastErr])
			d.logger.Printf(statTxt)
			result.Code = StatusErrorWritingToWS
			result.Text = statTxt
			return result
		}

		if errCnt > 0 && okCnt > 0 {
			format := "message id %s, from '%s', there were %d broadcasting errors. Last err: %s"
			statTxt := fmt.Sprintf(format, mesId, rAddr, errCnt, errs[iLastErr])
			d.logger.Printf(statTxt)
			result.Code = StatusOK
			result.Text = statTxt
			return result
		}

		result.Code = StatusOK
		result.Text = fmt.Sprintf("message id %s broadcasted successfully", mesId)
	} else {
		//send to specific device_id
		dev, found := d.getConn(devId)
		if !found {
			format := "message id %s, from '%s', device_id '%s' not registered"
			statTxt := fmt.Sprintf(format, mesId, rAddr, devId)
			d.logger.Println(statTxt)
			result.Code = StatusDeviceNotFound
			result.Text = statTxt
			return result
		}

		err := dev.writeWS(websocket.TextMessage, []byte(message))
		if err != nil {
			format := "message id %s, from '%s', error writing to device_id '%s'. Error: %s"
			statTxt := fmt.Sprintf(format, mesId, rAddr, devId, err)
			d.logger.Printf(statTxt)
			result.Code = StatusErrorWritingToWS
			result.Text = statTxt
			return result
		}

		result.Code = StatusOK
		result.Text = fmt.Sprintf("message id %s sent to %s", mesId, devId)

	}

	return result
}

func (dev *device) pingWS() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		dev.ws.Close()
		dev.wsHandler.delConn(dev.devId)
	}()

	for {
		select {
		case <-ticker.C:
			if err := dev.writeWS(websocket.PingMessage, nil); err != nil {
				dev.wsHandler.logger.Printf("error pinging websocket of device id %s. Closing websocket", dev.devId)
				return
			}
		case <-dev.done:
			return
		}

	}
}

func (dev *device) writeWS(mt int, data []byte) error {
	dev.wrMu.Lock()
	defer dev.wrMu.Unlock()
	dev.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return dev.ws.WriteMessage(mt, data)
}
