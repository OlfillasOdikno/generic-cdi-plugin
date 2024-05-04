package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"tags.cncf.io/container-device-interface/pkg/cdi"
)

type GenericCDIPlugin struct {
	resource string
	kind     string
	update   chan interface{}
	stop     chan interface{}
}

func (dp *GenericCDIPlugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {

	responses := &pluginapi.AllocateResponse{}
	for _, req := range r.ContainerRequests {
		devices := []*pluginapi.CDIDevice{}
		for _, id := range req.DevicesIDs {
			log.Printf("Got Allocate request for %s passing %s=%s", id, dp.kind, dp.resource)
			devices = append(devices, &pluginapi.CDIDevice{
				Name: fmt.Sprintf("%s=%s", dp.kind, dp.resource),
			})
		}
		responses.ContainerResponses = append(responses.ContainerResponses, &pluginapi.ContainerAllocateResponse{
			CDIDevices: devices,
		})
	}

	dp.update <- true
	return responses, nil
}

func (*GenericCDIPlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (*GenericCDIPlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (dp *GenericCDIPlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	log.Printf("Listening... %s=%s", dp.kind, dp.resource)
	for {
		devices := []*pluginapi.Device{}
		id := uuid.New()
		devices = append(devices, &pluginapi.Device{
			ID:     id.String(),
			Health: pluginapi.Healthy,
		})
		log.Printf("Registering device for %s=%s: %s", dp.kind, dp.resource, id.String())
		s.Send(&pluginapi.ListAndWatchResponse{
			Devices: devices,
		})
		select {
		case <-dp.stop:
			log.Printf("Stopping for %s=%s", dp.kind, dp.resource)
			return nil
		case <-dp.update:
			continue
		}
	}
}

func (*GenericCDIPlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (dp *GenericCDIPlugin) Stop() {
	dp.stop <- true
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
	return &GenericCDIPlugin{
		resource: resource,
		kind:     l.spec.Kind,
		update:   make(chan interface{}),
		stop:     make(chan interface{}),
	}
}

func main() {
	var cdiJSON string
	flag.StringVar(&cdiJSON, "str", "/var/run/cdi/nvidia-container-toolkit.json", "path to cdi json")
	spec, err := cdi.ReadSpec(cdiJSON, 0)
	if err != nil {
		panic(err)
	}
	dpm.NewManager(&GenericCDIPluginLister{
		spec: spec,
	}).Run()
}
