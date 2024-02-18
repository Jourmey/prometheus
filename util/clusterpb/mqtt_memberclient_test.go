package clusterpb

import (
	"context"
	"github.com/prometheus/prometheus/util/proto"
	"log"
	"testing"
	"time"
)

var (
	memberClient MemberClientAgent
)

func InitMqttMemberClient() {
	var (
		advertiseAddr = "127.0.0.1:4061"
	)

	c := NewMqttMemberClient(advertiseAddr)

	for {
		err := c.Connect()
		if err == nil {
			break
		}

		time.Sleep(5 * time.Second)
		log.Println("try connect again")
	}

	memberClient = c
}

func Test_MqttMemberClient_Request(t *testing.T) {

	InitMqttMemberClient()

	args := `[{"msg":"hello"}]`

	// sys.recover.msgRemote.forwardMessage
	res, err := memberClient.Request(context.Background(), proto.RequestRequest{
		Namespace:  "user",
		ServerType: "connector",
		Service:    "connectorRemote",
		Method:     "Test",
		Args:       []byte(args),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(string(res))
}
