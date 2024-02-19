package proto

import "encoding/json"

type MonitorAction string

const (
	Type_Monitor = "monitor"

	ServerType_Connector = "connector"
	ServerType_Chat      = "chat"
	ServerType_Recover   = "record"

	MonitorAction_addServer     MonitorAction = "addServer"
	MonitorAction_removeServer  MonitorAction = "removeServer"
	MonitorAction_replaceServer MonitorAction = "replaceServer"
	MonitorAction_startOve      MonitorAction = "startOver"
)

const (
	BEFORE_FILTER        = "__befores__"
	AFTER_FILTER         = "__afters__"
	GLOBAL_BEFORE_FILTER = "__globalBefores__"
	GLOBAL_AFTER_FILTER  = "__globalAfters__"
	ROUTE                = "__routes__"
	BEFORE_STOP_HOOK     = "__beforeStopHook__"
	MODULE               = "__modules__"
	SERVER_MAP           = "__serverMap__"
	RPC_BEFORE_FILTER    = "__rpcBefores__"
	RPC_AFTER_FILTER     = "__rpcAfters__"
	MASTER_WATCHER       = "__masterwatcher__"
	MONITOR_WATCHER      = "__monitorwatcher__"
)

// ClusterServerInfo 集群服务信息
type ClusterServerInfo map[string]interface{}

// Register 向master注册服务信息
type (
	RegisterRequest struct {
		ServerInfo ClusterServerInfo

		Token string
	}

	RegisterResponse struct{}
)

// Subscribe 订阅master中集群信息
type (
	SubscribeRequest struct {
		Id string `json:"id"`
	}

	SubscribeResponse map[string]ClusterServerInfo // 集群内其他服务信息
)

// Record 通知master启动完毕
type (
	RecordRequest struct {
		Id string `json:"id"`
	}

	RecordResponse struct{}
)

// MonitorHandler 监听master中的集群变化
type (
	MonitorHandlerRequest struct {
		AddServerCallBackHandler    func(serverInfos []ClusterServerInfo) // 新增服务
		RemoveServerCallBackHandler func(id string)                       // 删除服务
	}

	MonitorHandlerResponse struct{}
)

// Request 发送Request rpc请求
type (
	RequestRequest struct {
		Namespace  string          `json:"namespace"`
		ServerType string          `json:"serverType"`
		Service    string          `json:"service"`
		Method     string          `json:"method"`
		Args       json.RawMessage `json:"args"` // []interface{}{}
	}

	RequestResponse json.RawMessage // []interface{}{}
)

type Session struct {
	Id         int     `json:"id"`
	FrontendId string  `json:"frontendId"`
	Uid        string  `json:"uid"`
	Settings   Setting `json:"settings"`
}

type Setting struct {
	UniqId    string `json:"uniqId"`
	Rid       string `json:"rid"`
	Rtype     int    `json:"rtype"`
	Role      int    `json:"role"`
	Ulevel    int    `json:"ulevel"`
	Uname     string `json:"uname"`
	Classid   string `json:"classid"`
	ClientVer string `json:"clientVer"`
	UserVer   string `json:"userVer"`
	LiveType  int    `json:"liveType"`
}

// Message 消息体
type Message struct {
	Id            int             `json:"id"`            // 消息的唯一标识符
	Type          int             `json:"type"`          // 消息类型
	CompressRoute int             `json:"compressRoute"` // 路由压缩标志，用于指示是否对路由信息进行了压缩
	Route         string          `json:"route"`         // 路由信息，指明消息的目的地
	CompressGzip  int             `json:"compressGzip"`  // Gzip压缩标志，用于指示消息体是否使用Gzip进行了压缩
	Body          json.RawMessage `json:"body"`          // 消息体，存储实际的消息内容，使用json.RawMessage类型以便于后续处理
}
