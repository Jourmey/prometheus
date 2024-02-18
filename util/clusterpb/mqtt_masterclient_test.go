package clusterpb

import (
	"context"
	"encoding/json"
	"github.com/prometheus/prometheus/util/proto"
	"log"
	"strconv"
	"testing"
	"time"
)

var (
	advertiseAddr = "10.108.228.154:3005"
	serverId      = "cluster-server-connector-994"

	request = &proto.RegisterRequest{
		ServerInfo: proto.ClusterServerInfo{
			"serverType": proto.ServerType_Chat,
			"id":         serverId,
			"type":       proto.Type_Monitor,
			"pid":        99,
			"info": map[string]interface{}{
				"serverType": proto.ServerType_Chat,
				"id":         serverId,
				"env":        "local",
				"host":       "127.0.0.1",
				"port":       4061,

				"channelType":   2,
				"cloudType":     1,
				"clusterCount":  1,
				"restart-force": "true",
			},
		},
		Token: "agarxhqb98rpajloaxn34ga8xrunpagkjwlaw3ruxnpaagl29w4rxn",
	}

	client MasterClientAgent
)

func Init() {
	c := NewMqttMasterClient(advertiseAddr)

	for {
		err := c.Connect()
		if err == nil {
			break
		}

		time.Sleep(5 * time.Second)
		log.Println("try connect again")
	}

	client = c
}

func Test_MqttMasterClient_All(t *testing.T) {
	Init()

	Test_MqttMasterClient_Register(t)

	Test_MqttMasterClient_Subscribe(t)

	Test_MqttMasterClient_Record(t)

	Test_MqttMasterClient_MonitorHandler(t)

	select {}
}

func Test_MqttMasterClient_Register(t *testing.T) {

	res, err := client.Register(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(res)
}

func Test_MqttMasterClient_Subscribe(t *testing.T) {

	res, err := client.Subscribe(context.Background(), &proto.SubscribeRequest{
		Id: serverId,
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Log(res)
}

func Test_MqttMasterClient_Record(t *testing.T) {

	res, err := client.Record(context.Background(), &proto.RecordRequest{
		Id: serverId,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(res)
}

func Test_MqttMasterClient_MonitorHandler(t *testing.T) {

	res, err := client.MonitorHandler(context.Background(), &proto.MonitorHandlerRequest{
		CallBackHandler: func(action proto.MonitorAction, serverInfos []proto.ClusterServerInfo) {
			t.Log(action)
			t.Log(serverInfos)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(res)
}

func Test_monitorMessage(t *testing.T) {

	s0 := `"{\"reqId\":53,\"moduleId\":\"__monitorwatcher__\",\"body\":{\"action\":\"addServer\",\"server\":[{\"channelType\":2,\"clientPort\":3061,\"cloudType\":1,\"clusterCount\":1,\"env\":\"local\",\"frontend\":\"true\",\"host\":\"127.0.0.1\",\"id\":\"cluster-server-connector-998\",\"port\":4061,\"record\":\"true\",\"restart-force\":\"true\",\"serverType\":\"connector\",\"wssPort\":80,\"pid\":99}]}}"`

	// 这里接收的字符串居然是转义后的
	unescapedString, err := strconv.Unquote(s0)
	if err != nil {
		t.Fatal(err)
	}

	//respId, err := jsonparser.GetInt([]byte(unescapedString), "reqId")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//t.Log(respId)

	msg := monitorMessage{}

	err = json.Unmarshal([]byte(unescapedString), &msg)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(msg)
}
