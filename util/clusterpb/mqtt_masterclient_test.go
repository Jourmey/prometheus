package clusterpb

import (
	"context"
	"encoding/json"
	"github.com/prometheus/prometheus/util/proto"
	"log"
	"reflect"
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

	res, err := client.MonitorHandler(context.Background(), &proto.MonitorHandlerRequest{})
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

func Test_analysisClusterServerInfo1(t *testing.T) {
	type args struct {
		in string
	}
	tests := []struct {
		name    string
		args    args
		wantOut []proto.ClusterServerInfo
	}{
		{
			name: "t1",
			args: args{
				in: `[
    {
      "channelType": 2,
      "cloudType": 1,
      "clusterCount": 1,
      "env": "local",
      "host": "127.0.0.1",
      "id": "cluster-server-monitor-001",
      "port": 4061,
      "restart-force": "true",
      "serverType": "monitor",
      "pid": 99
    }
  ]`,
			},
			wantOut: []proto.ClusterServerInfo{
				{
					"channelType":   2,
					"cloudType":     1,
					"clusterCount":  1,
					"env":           "local",
					"host":          "127.0.0.1",
					"id":            "cluster-server-monitor-001",
					"port":          4061,
					"restart-force": "true",
					"serverType":    "monitor",
					"pid":           99,
				},
			},
		},
		{
			name: "t2",
			args: args{
				in: `{
    "main": "/app/app.js",
    "env": "production",
    "host": "10.108.231.103",
    "port": 12000,
    "channelType": 2,
    "clusterCount": 1,
    "restart-force": "false",
    "recover": "true",
    "delay-notify": "true",
    "serverType": "chat",
    "id": "cluster-server-chat-2-10.108.231.103-1708254932",
    "pid": 16
  }`,
			},
			wantOut: []proto.ClusterServerInfo{{
				"main":          "/app/app.js",
				"env":           "production",
				"host":          "10.108.231.103",
				"port":          12000,
				"channelType":   2,
				"clusterCount":  1,
				"restart-force": "false",
				"recover":       "true",
				"delay-notify":  "true",
				"serverType":    "chat",
				"id":            "cluster-server-chat-2-10.108.231.103-1708254932",
				"pid":           16,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var server interface{}

			err := json.Unmarshal([]byte(tt.args.in), &server)
			if err != nil {
				t.Fatal(err)
			}
			if gotOut := analysisClusterServerInfo(server); !reflect.DeepEqual(gotOut, tt.wantOut) {
				t.Errorf("analysisClusterServerInfo() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}
