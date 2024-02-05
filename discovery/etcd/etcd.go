package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	clientv3 "go.etcd.io/etcd/client/v3"
	"os"
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
	Endpoints   []string `yaml:"endpoints,omitempty"`
	Prefixs     []string `yaml:"prefixs,omitempty"`
	CertFile    string   `yaml:"certFile,omitempty"`
	CertKeyFile string   `yaml:"certKeyFile,omitempty"`
	CaFile      string   `yaml:"caFile,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "etcd" }

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
	cli *clientv3.Client //etcd client
	cfg SDConfig

	registerInfo map[string]*sync.Map // key value etcd中的原始数据
	logger       log.Logger
}

// NewDiscovery returns a new Discovery for the given config.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	t, err := addTLS(conf.CertFile, conf.CertKeyFile, conf.CaFile, false)
	if err != nil {
		return nil, err
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   conf.Endpoints,
		DialTimeout: 5 * time.Second,
		TLS:         t,
	})
	if err != nil {
		return nil, err
	}

	registerInfo := make(map[string]*sync.Map, len(conf.Prefixs)) // key value etcd中的原始数据
	for i := range conf.Prefixs {
		registerInfo[conf.Prefixs[i]] = &sync.Map{}
	}

	cd := &Discovery{
		cli:          cli,
		cfg:          *conf,
		registerInfo: registerInfo,
		logger:       logger,
	}
	return cd, nil
}

func addTLS(certFile, certKeyFile, caFile string, insecureSkipVerify bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, certKeyFile)
	if err != nil {
		return nil, err
	}

	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caData)

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            pool,
		InsecureSkipVerify: insecureSkipVerify,
	}, nil

}

// Initialize the Discoverer run.
func (d *Discovery) initialize(ctx context.Context, prefix string, up chan<- []*targetgroup.Group) {

	// Loop until we manage to get the local datacenter.
	for {
		// We have to check the context at least once. The checks during channel sends
		// do not guarantee that.
		select {
		case <-ctx.Done():
			return
		default:
		}

		res, err := d.cli.Get(ctx, prefix, clientv3.WithPrefix())
		if err != nil {
			time.Sleep(retryInterval)
			continue
		}

		m, ok := d.registerInfo[prefix]
		if !ok {
			return
		}

		for i := range res.Kvs {
			m.Store(string(res.Kvs[i].Key), res.Kvs[i].Value)
		}

		groups := d.analysisGroup(prefix)

		up <- []*targetgroup.Group{groups}
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

	for prefix := range d.registerInfo {
		d.initialize(ctx, prefix, up)
	}

	for prefix := range d.registerInfo {
		go d.watchService(ctx, up, prefix)
	}

	<-ctx.Done()

}

// Start watching a service.
func (d *Discovery) watchService(ctx context.Context, ch chan<- []*targetgroup.Group, prefix string) bool {

	rch := d.cli.Watch(ctx, prefix, clientv3.WithPrefix())
	for {
		select {
		case wresp, ok := <-rch:
			if !ok {
				return false
			}
			if wresp.Canceled {
				return false
			}
			if wresp.Err() != nil {
				return false
			}

			m, ok := d.registerInfo[prefix]
			if !ok {
				return false
			}

			for _, ev := range wresp.Events {
				switch ev.Type {
				case clientv3.EventTypePut:
					m.Store(string(ev.Kv.Key), ev.Kv.Value)

				case clientv3.EventTypeDelete:
					m.Delete(string(ev.Kv.Key))
				default:
					_ = d.logger.Log("Unknown event type: ", ev.Type)
				}
			}

			group := d.analysisGroup(prefix)
			ch <- []*targetgroup.Group{group}

		case <-ctx.Done():
			return true
		}
	}
}
