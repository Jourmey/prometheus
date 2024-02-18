package pomelo

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/clusterpb"
	"github.com/prometheus/prometheus/util/proto"
	"sync"
	"time"
)

const (
	watchTimeout  = 2 * time.Minute
	retryInterval = 15 * time.Second
)

var (

	// DefaultSDConfig is the default Consul SD configuration.
	DefaultSDConfig = SDConfig{}
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Consul service discovery.
type SDConfig struct {
	ServerId      string   `yaml:"serverId"` // 本服务serverid
	Servers       []string `yaml:"servers"`
	AdvertiseAddr string   `yaml:"advertiseAddr"` // node服务对应的master地址
	Token         string   `yaml:"token"`         // master 通信token
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "pomelo" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	//c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return nil
}

// Discovery retrieves target information from a Consul server
// and updates them via watches.
type Discovery struct {
	cfg SDConfig

	registerInfo     map[string]*sync.Map        // key value etcd中的原始数据
	mqttMasterClient clusterpb.MasterClientAgent // pomelo master agent
	logger           log.Logger
}

// NewDiscovery returns a new Discovery for the given config.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	registerInfo := make(map[string]*sync.Map, len(conf.Servers)) // key value etcd中的原始数据
	for i := range conf.Servers {
		registerInfo[conf.Servers[i]] = &sync.Map{}
	}

	mqttMasterClient := clusterpb.NewMqttMasterClient(conf.AdvertiseAddr)

	for {
		err := mqttMasterClient.Connect()
		if err == nil {
			break
		}

		time.Sleep(5 * time.Second)
	}

	_, err := mqttMasterClient.Register(context.Background(), &proto.RegisterRequest{
		ServerInfo: proto.ClusterServerInfo{
			"serverType": "monitor", // proto.ServerType_Chat,
			"id":         conf.ServerId,
			"type":       proto.Type_Monitor,
			"pid":        99,
			"info": map[string]interface{}{
				"serverType": "monitor", //proto.ServerType_Chat,
				"id":         conf.ServerId,
				"env":        "local",
				"host":       "127.0.0.1",
				"port":       4061,

				"channelType":   2, // 很关键的参数 否则注册不了
				"cloudType":     1,
				"clusterCount":  1,
				"restart-force": "true",
			},
		},
		Token: conf.Token,
	})
	if err != nil {
		return nil, err
	}

	cd := &Discovery{
		cfg:              *conf,
		registerInfo:     registerInfo,
		mqttMasterClient: mqttMasterClient,
		logger:           logger,
	}
	return cd, nil
}

// Initialize the Discoverer run.
func (d *Discovery) initialize(ctx context.Context, up chan<- []*targetgroup.Group) {

	// Loop until we manage to get the local datacenter.
	for {
		// We have to check the context at least once. The checks during channel sends
		// do not guarantee that.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 获取注册信息
		subscribeResponse, err := d.mqttMasterClient.Subscribe(context.Background(), &proto.SubscribeRequest{
			Id: d.cfg.ServerId,
		})
		if err != nil {
			_ = d.logger.Log("mqttMasterClient.Subscribe failed,retrying... , err:", err)

			time.Sleep(retryInterval)
			continue
		}

		for key, info := range *subscribeResponse {

			if serverType, ok := info["serverType"]; ok {
				if store, ok := d.registerInfo[serverType.(string)]; ok {
					store.Store(key, info)
				}
			}
		}

		gs := make([]*targetgroup.Group, 0, len(d.registerInfo))

		for key := range d.registerInfo {
			groups := d.analysisGroup(key)
			gs = append(gs, groups)
		}

		up <- gs
		// We are good to go.
		return
	}
}

func (d *Discovery) analysisGroup(prefix string) (group *targetgroup.Group) {

	group = &targetgroup.Group{
		Targets: make([]model.LabelSet, 0),
		Labels:  model.LabelSet{},
		Source:  prefix,
	}

	m, ok := d.registerInfo[prefix]
	if !ok {
		return group
	}

	m.Range(func(key, value any) bool {

		data := value.(proto.ClusterServerInfo)

		target := model.LabelSet{
			model.AddressLabel:    model.LabelValue(fmt.Sprintf("%s:%d", data["host"], data["port"])),
			model.MetricNameLabel: model.LabelValue(fmt.Sprintf("%s", data["id"])),
		}

		group.Targets = append(group.Targets, target)

		return true
	})

	return group
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, up chan<- []*targetgroup.Group) {

	_, err := d.mqttMasterClient.MonitorHandler(ctx, &proto.MonitorHandlerRequest{
		CallBackHandler: func(action proto.MonitorAction, serverInfos []proto.ClusterServerInfo) {

		},
	})
	if err != nil {
		return
	}

	d.initialize(ctx, up)

	<-ctx.Done()
}
