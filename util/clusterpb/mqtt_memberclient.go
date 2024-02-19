package clusterpb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/util/proto"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	topic_RPC = "rpc"
)

type MqttMemberClient struct {
	clientId string // = 'MQTT_RPC_' + Date.now();

	advertiseAddr  string
	keepaliveTimer time.Duration // default 2s
	pingTimeout    time.Duration // default 1s
	requestTimeout time.Duration // default 10s

	reqId      int
	reqIdMutex sync.Mutex

	socket mqtt.Client
	resp   sync.Map // monitor memberRequest 请求列表
}

func (m *MqttMemberClient) Request(ctx context.Context, in proto.RequestRequest) (out proto.RequestResponse, err error) {

	m.reqIdMutex.Lock()
	m.reqId++
	var reqId = m.reqId
	m.reqIdMutex.Unlock()

	err = m.doSend(topic_RPC, rpcRequestMessageRequest{
		Id:  reqId,
		Msg: &in,
	})

	if err != nil {
		return nil, err
	}

	r := memberRequest{
		resp:  make(chan rpcMessageResponse),
		reqId: reqId,
	}

	m.resp.Store(reqId, r)

	select {
	case resp := <-r.resp:
		// &[<nil> map[interactMode:0 length:1 mainTeacherClientVer:2.9.8.7 users:[]]]
		return proto.RequestResponse(resp.Resp), nil

	case <-time.After(m.requestTimeout):
		return nil, errors.New("timeout")
	}

}

func (m *MqttMemberClient) Connect() error {

	token := m.socket.Connect()

	token.Wait()

	return token.Error()
}

func (m *MqttMemberClient) Close() error {
	return nil
}

func (m *MqttMemberClient) doSend(topic string, msg interface{}) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if pToken := m.socket.Publish(topic, 0, false, payload); pToken.Wait() && pToken.Error() != nil {
		return pToken.Error()
	}

	return nil
}

func (m *MqttMemberClient) publishHandler(client mqtt.Client, message mqtt.Message) {

	switch message.Topic() {

	case topic_RPC:

		msg := rpcMessageResponse{}

		err := json.Unmarshal(message.Payload(), &msg)
		if err != nil {
			return
		}

		req, ok := m.resp.LoadAndDelete(msg.Id)
		if !ok {
			return
		}

		mReq := req.(memberRequest)

		select {
		case mReq.resp <- msg:
			close(mReq.resp)
		default:
		}

	default:

	}

}

func NewMqttMemberClient(advertiseAddr string) MemberClientAgent {

	var (
		clientId       = fmt.Sprintf("MQTT_RPC_%d", time.Now().UnixMilli())
		keepaliveTimer = 2 * time.Second
		pingTimeout    = 1 * time.Second
		requestTimeout = 5 * time.Second
	)

	m := &MqttMemberClient{
		clientId:       clientId,
		advertiseAddr:  advertiseAddr,
		keepaliveTimer: keepaliveTimer,
		pingTimeout:    pingTimeout,
		requestTimeout: requestTimeout,
		reqId:          0,
		socket:         nil,
		resp:           sync.Map{},
	}

	opts := mqtt.NewClientOptions().
		AddBroker(advertiseAddr).
		SetClientID(m.clientId).
		SetCleanSession(false).
		SetIgnoreVerifyConnACK(true) // 这里对mqtt做了适配改造，pomelo的服务端不会回复connACK，正常逻辑导致连接失败

	//opts.SetKeepAlive(m.keepaliveTimer)
	opts.SetDefaultPublishHandler(m.publishHandler)
	opts.SetPingTimeout(m.pingTimeout)

	socket := mqtt.NewClient(opts)
	m.socket = socket

	return m
}

type memberRequest struct {
	resp  chan rpcMessageResponse
	reqId int
}

type (
	// rpc请求的结构
	rpcRequestMessageRequest struct {
		Id  int                   `json:"id"`
		Msg *proto.RequestRequest `json:"msg"`
	}

	// rpc响应的返回值结构
	rpcMessageResponse struct {
		Id   int             `json:"id"`   //  "respId": 1,
		Resp json.RawMessage `json:"resp"` // 不同返回值的
	}
)