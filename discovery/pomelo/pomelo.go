package pomelo

import (
	"context"
	"encoding/json"
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
			time.Sleep(retryInterval)
			continue
		}

		for key, info := range *subscribeResponse {
			if store, ok := d.registerInfo[key]; ok {

				store.Store(key, info)
			}
		}

		//groups := d.analysisGroup(prefix)

		//up <- []*targetgroup.Group{groups}
		// We are good to go.
		return
	}
}

func (d *Discovery) analysisGroup(prefix string) (group *targetgroup.Group) {

	type etcdRegister struct {
		Main         string `json:"main"`
		Env          string `json:"env"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		ClusterCount int    `json:"clusterCount"`
		Topic        string `json:"topic"`
		Group        string `json:"group"`
		RestartForce string `json:"restart-force"`
		ServerType   string `json:"serverType"`
		Id           string `json:"id"`
	}

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

		register := etcdRegister{}
		data := value.([]byte)

		err := json.Unmarshal(data, &register)
		if err != nil {
			return true
		}

		target := model.LabelSet{
			model.AddressLabel:    model.LabelValue(fmt.Sprintf("%s:%d", register.Host, register.Port)),
			model.MetricNameLabel: model.LabelValue(register.Id),
			"topic":               model.LabelValue(register.Topic),
			"group":               model.LabelValue(register.Group),
		}

		group.Targets = append(group.Targets, target)

		return true
	})

	return group
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, up chan<- []*targetgroup.Group) {

	d.initialize(ctx, up)

	for prefix := range d.registerInfo {
		go d.watchService(ctx, up, prefix)
	}

	<-ctx.Done()

}

// Start watching a service.
func (d *Discovery) watchService(ctx context.Context, ch chan<- []*targetgroup.Group, prefix string) bool {

	return false
}
