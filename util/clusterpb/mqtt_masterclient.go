package clusterpb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/prometheus/util/proto"
)

const (
	topic_Register = "register"
	topic_Monitor  = "monitor"

	action_Subscribe = "subscribe"
	action_Record    = "record"

	pro_ok   = 1
	pro_fail = -1
)

type MqttMasterClient struct {
	clientId string // = 'MQTT_ADMIN_' + Date.now();

	advertiseAddr  string
	keepaliveTimer time.Duration // default 2s
	pingTimeout    time.Duration // default 1s
	requestTimeout time.Duration // default 10s

	reqId       int
	socket      mqtt.Client
	monitorResp sync.Map // monitor request 请求列表

	addServerCallBackHandler    func(serverInfos []proto.ClusterServerInfo) // 新增服务
	removeServerCallBackHandler func(id string)                             // 删除服务

	register  chan registerResponse
	subscribe chan proto.ClusterServerInfo
}

func (m *MqttMasterClient) Register(ctx context.Context, in *proto.RegisterRequest) (*proto.RegisterResponse, error) {

	req := make(map[string]interface{}, len(in.ServerInfo)+1)

	for s, i := range in.ServerInfo {
		req[s] = i
	}

	req["token"] = in.Token

	err := m.doSend(topic_Register, req)
	if err != nil {
		return nil, err
	}

	select {
	case res := <-m.register:

		if res.Code == pro_ok {
			return &proto.RegisterResponse{}, nil
		}

		return nil, errors.New(res.Msg)

	case <-time.After(m.requestTimeout):
		return nil, errors.New("receive register timeout")
	}

}

func (m *MqttMasterClient) Subscribe(ctx context.Context, in *proto.SubscribeRequest) (*proto.SubscribeResponse, error) {

	request := subscribeRequest{
		Action: action_Subscribe,
		Id:     in.Id,
	}

	response, err := m.request(proto.MASTER_WATCHER, request)
	if err != nil {
		return nil, err
	}

	res := proto.SubscribeResponse{}

	err = json.Unmarshal(response.Body, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (m *MqttMasterClient) Record(ctx context.Context, in *proto.RecordRequest) (*proto.RecordResponse, error) {

	var msg = recordRequest{
		Action: action_Record,
		Id:     in.Id,
	}

	err := m.notify(proto.MASTER_WATCHER, msg)

	return &proto.RecordResponse{}, err
}

func (m *MqttMasterClient) MonitorHandler(ctx context.Context, in *proto.MonitorHandlerRequest) (*proto.MonitorHandlerResponse, error) {

	m.addServerCallBackHandler = in.AddServerCallBackHandler
	m.removeServerCallBackHandler = in.RemoveServerCallBackHandler

	return &proto.MonitorHandlerResponse{}, nil
}

func (m *MqttMasterClient) Connect() error {

	token := m.socket.Connect()

	token.Wait()

	return token.Error()
}

func (m *MqttMasterClient) Close() error {
	return nil
}

func (m *MqttMasterClient) publishHandler(client mqtt.Client, message mqtt.Message) {

	switch message.Topic() {

	case topic_Register:

		res := registerResponse{}
		err := json.Unmarshal(message.Payload(), &res)
		if err != nil {
			return
		}

		select {
		case m.register <- res:
		default:
		}

	case topic_Monitor:

		msg := monitorMessage{}

		// 这里接收的字符串居然是转义后的
		unescapedString, err := strconv.Unquote(string(message.Payload()))
		if err != nil {
			return
		}

		err = json.Unmarshal([]byte(unescapedString), &msg)
		if err != nil {
			return
		}

		if msg.Command != nil {

		} else if msg.RespId != nil {

			req, ok := m.monitorResp.LoadAndDelete(*msg.RespId)
			if !ok {
				return
			}
			mReq := req.(monitorRequest)

			select {
			case mReq.resp <- msg:
				close(mReq.resp)
			default:
			}

		} else {

			type monitorMessageOnChangeBody struct {
				Action proto.MonitorAction `json:"action"`
				Server interface{}         `json:"server"` // 这里的server可能是数组 可能是对象 需要特殊处理
				Id     string              `json:"id"`
			}

			body := monitorMessageOnChangeBody{}

			log.Println("MqttMasterClient topic_Monitor change, msg body:", string(msg.Body))

			err := json.Unmarshal(msg.Body, &body)
			if err != nil {
				log.Println("MqttMasterClient topic_Monitor json.Unmarshal failed,err:", err)
				return
			}

			switch body.Action {
			case proto.MonitorAction_addServer:

				if m.addServerCallBackHandler != nil {
					m.addServerCallBackHandler(analysisClusterServerInfo(body.Server))
				}

			case proto.MonitorAction_removeServer:

				if m.removeServerCallBackHandler != nil {
					m.removeServerCallBackHandler(body.Id)
				}

			case proto.MonitorAction_replaceServer:
			case proto.MonitorAction_startOve:
			}
		}

	default:

	}

}

func analysisClusterServerInfo(in interface{}) (out []proto.ClusterServerInfo) {

	switch server := in.(type) {

	case map[string]interface{}:

		out = append(out, server)

	case []interface{}:

		for _, s := range server {
			switch one := s.(type) {

			case map[string]interface{}:
				out = append(out, one)
			}
		}
	}

	return out
}

func (m *MqttMasterClient) notify(moduleId string, body interface{}) error {
	return m.doSend(topic_Monitor, map[string]interface{}{
		"moduleId": moduleId,
		"body":     body,
	})
}

// 同步请求
func (m *MqttMasterClient) request(moduleId string, body interface{}) (res monitorMessage, err error) {

	m.reqId++
	var reqId = m.reqId
	err = m.doSend(topic_Monitor, map[string]interface{}{
		"reqId":    reqId,
		"moduleId": moduleId,
		"body":     body,
	})

	if err != nil {
		return monitorMessage{}, err
	}

	r := monitorRequest{
		resp:  make(chan monitorMessage),
		reqId: reqId,
	}

	m.monitorResp.Store(reqId, r)

	select {
	case res = <-r.resp:
		return res, nil

	case <-time.After(m.requestTimeout):
		return monitorMessage{}, errors.New("timeout")
	}

}

func (m *MqttMasterClient) doSend(topic string, msg interface{}) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if pToken := m.socket.Publish(topic, 0, false, payload); pToken.Wait() && pToken.Error() != nil {
		return pToken.Error()
	}

	return nil
}

func NewMqttMasterClient(advertiseAddr string) MasterClientAgent {

	var (
		clientId       = fmt.Sprintf("MQTT_ADMIN_%d", time.Now().UnixMilli())
		keepaliveTimer = 2 * time.Second
		pingTimeout    = 1 * time.Second
		requestTimeout = 5 * time.Second
	)

	m := &MqttMasterClient{
		clientId:       clientId,
		advertiseAddr:  advertiseAddr,
		keepaliveTimer: keepaliveTimer,
		pingTimeout:    pingTimeout,
		requestTimeout: requestTimeout,
		reqId:          0,
		socket:         nil,
		monitorResp:    sync.Map{},
		register:       make(chan registerResponse),
		subscribe:      make(chan proto.ClusterServerInfo),
	}

	opts := mqtt.NewClientOptions().
		AddBroker(advertiseAddr).
		SetClientID(m.clientId)

	opts.SetKeepAlive(m.keepaliveTimer)
	opts.SetDefaultPublishHandler(m.publishHandler)
	opts.SetPingTimeout(m.pingTimeout)

	socket := mqtt.NewClient(opts)
	m.socket = socket

	return m
}

type monitorRequest struct {
	resp  chan monitorMessage
	reqId int
}

type monitorMessage struct {
	RespId *int    `json:"respId"` //  "respId": 1,
	Error  *string `json:"error"`  //  "error": null,

	ReqId    *int    `json:"reqId"`    //  "reqId": 1,
	ModuleId *string `json:"moduleId"` //  "moduleId": "__monitorwatcher__",

	Command *string `json:"command"` // command

	Body json.RawMessage `json:"body"` // 不同返回值的
}

type registerResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type subscribeRequest struct {
	Action string `json:"action"`
	Id     string `json:"id"`
}

type recordRequest struct {
	Action string `json:"action"`
	Id     string `json:"id"`
}