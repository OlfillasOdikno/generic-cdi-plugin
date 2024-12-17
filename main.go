package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	"tags.cncf.io/container-device-interface/pkg/cdi"
)

type GenericCDIPlugin struct {
	resource string
	kind     string
	update   chan interface{}
	stop     chan interface{}
	client   podresourcesv1.PodResourcesListerClient
	conn     *grpc.ClientConn
	mu       sync.Mutex
	devices  []*pluginapi.Device
}

func (dp *GenericCDIPlugin) printf(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	log.Printf("%s=%s: %s", dp.kind, dp.resource, s)

}
func (dp *GenericCDIPlugin) createDevice() {
	id := uuid.New()
	dp.devices = append(dp.devices, &pluginapi.Device{
		ID:     id.String(),
		Health: pluginapi.Healthy,
	})
	dp.printf("created new device: %s", id.String())
}

func (dp *GenericCDIPlugin) collectGarbage() {
	resource := fmt.Sprintf("%s-%s", dp.kind, dp.resource)

	dp.mu.Lock()
	dp.printf("collecting garbage...")
	start := time.Now()

	resp, err := dp.client.List(context.Background(), &podresourcesv1.ListPodResourcesRequest{})
	if err != nil {
		log.Fatalf("Failed to list pod resources: %v", err)
	}

	newDevices := []*pluginapi.Device{}
	for _, res := range resp.PodResources {
		for _, cont := range res.Containers {
			for _, dev := range cont.Devices {
				if dev.ResourceName != resource {
					continue
				}
				for _, deviceID := range dev.DeviceIds {
					newDevices = append(newDevices, &pluginapi.Device{
						ID:     deviceID,
						Health: pluginapi.Healthy,
					})
				}
			}
		}
	}
	dp.devices = newDevices
	dp.createDevice()
	duration := time.Since(start)
	dp.printf("garbage collection took %v seconds", duration.Seconds())
	dp.mu.Unlock()
	dp.update <- true
}

func (dp *GenericCDIPlugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := &pluginapi.AllocateResponse{}
	for _, req := range r.ContainerRequests {
		devices := []*pluginapi.CDIDevice{}
		for _, id := range req.DevicesIDs {
			dp.printf("got Allocate request: %s", id)

			devices = append(devices, &pluginapi.CDIDevice{
				Name: fmt.Sprintf("%s=%s", dp.kind, dp.resource),
			})
		}
		responses.ContainerResponses = append(responses.ContainerResponses, &pluginapi.ContainerAllocateResponse{
			CDIDevices: devices,
		})
	}

	dp.mu.Lock()
	dp.createDevice()
	dp.mu.Unlock()
	dp.update <- true
	return responses, nil
}

func (*GenericCDIPlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired:                false,
		GetPreferredAllocationAvailable: false,
	}, nil
}

func (*GenericCDIPlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (dp *GenericCDIPlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	dp.printf("listening...")
	for {
		s.Send(&pluginapi.ListAndWatchResponse{
			Devices: dp.devices,
		})
		select {
		case <-dp.stop:
			dp.printf("stopping.")
			return nil
		case <-dp.update:
			continue
		}
	}
}

func (*GenericCDIPlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (dp *GenericCDIPlugin) Start() error {
	dp.createDevice()

	go func(dp *GenericCDIPlugin) {
		for {
			select {
			case <-dp.stop:
				dp.printf("stopping garbage collector.")
				return
			case <-time.After(30 * time.Second):
				dp.collectGarbage()
			}
		}
	}(dp)

	return nil
}

func (dp *GenericCDIPlugin) Stop() error {
	dp.stop <- true
	dp.conn.Close()

	return nil
}

type GenericCDIPluginLister struct {
	spec *cdi.Spec
}

func (l *GenericCDIPluginLister) Discover(pluginListCh chan dpm.PluginNameList) {
	plugins := dpm.PluginNameList{}
	for _, device := range l.spec.Devices {
		plugins = append(plugins, fmt.Sprintf("%s-%s", l.spec.GetClass(), device.Name))
	}
	pluginListCh <- plugins
}

func (l *GenericCDIPluginLister) GetResourceNamespace() string {
	return l.spec.GetVendor()
}

func (l *GenericCDIPluginLister) NewPlugin(name string) dpm.PluginInterface {
	resource := name[len(l.spec.GetClass())+1:]
	log.Printf("Registering plugin for %s=%s", l.spec.Kind, resource)

	client, conn, err := podresources.GetV1Client("unix:///var/lib/kubelet/pod-resources/kubelet.sock", 60*time.Second, 1024*1024)
	if err != nil {
		log.Fatalf("Failed to connect to pod-resources kubelet: %v", err)
	}

	return &GenericCDIPlugin{
		resource: resource,
		kind:     l.spec.Kind,
		update:   make(chan interface{}),
		stop:     make(chan interface{}),
		client:   client,
		conn:     conn,
		devices:  []*pluginapi.Device{},
	}
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("No path to CDI JSON provided. Exiting.")
	}
	cdiJSON := flag.Arg(0)

	spec, err := cdi.ReadSpec(cdiJSON, 0)
	if err != nil {
		panic(err)
	}
	dpm.NewManager(&GenericCDIPluginLister{
		spec: spec,
	}).Run()
}
