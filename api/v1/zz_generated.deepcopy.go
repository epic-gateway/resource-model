//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	"github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Account) DeepCopyInto(out *Account) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Account.
func (in *Account) DeepCopy() *Account {
	if in == nil {
		return nil
	}
	out := new(Account)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Account) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccountList) DeepCopyInto(out *AccountList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Account, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccountList.
func (in *AccountList) DeepCopy() *AccountList {
	if in == nil {
		return nil
	}
	out := new(AccountList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AccountList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccountSpec) DeepCopyInto(out *AccountSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccountSpec.
func (in *AccountSpec) DeepCopy() *AccountSpec {
	if in == nil {
		return nil
	}
	out := new(AccountSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccountStatus) DeepCopyInto(out *AccountStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccountStatus.
func (in *AccountStatus) DeepCopy() *AccountStatus {
	if in == nil {
		return nil
	}
	out := new(AccountStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientRef) DeepCopyInto(out *ClientRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientRef.
func (in *ClientRef) DeepCopy() *ClientRef {
	if in == nil {
		return nil
	}
	out := new(ClientRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPIC) DeepCopyInto(out *EPIC) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPIC.
func (in *EPIC) DeepCopy() *EPIC {
	if in == nil {
		return nil
	}
	out := new(EPIC)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EPIC) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPICEndpointMap) DeepCopyInto(out *EPICEndpointMap) {
	*out = *in
	if in.EPICEndpoints != nil {
		in, out := &in.EPICEndpoints, &out.EPICEndpoints
		*out = make(map[string]GUETunnelEndpoint, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPICEndpointMap.
func (in *EPICEndpointMap) DeepCopy() *EPICEndpointMap {
	if in == nil {
		return nil
	}
	out := new(EPICEndpointMap)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPICList) DeepCopyInto(out *EPICList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]EPIC, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPICList.
func (in *EPICList) DeepCopy() *EPICList {
	if in == nil {
		return nil
	}
	out := new(EPICList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EPICList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPICSpec) DeepCopyInto(out *EPICSpec) {
	*out = *in
	if in.XDSImage != nil {
		in, out := &in.XDSImage, &out.XDSImage
		*out = new(string)
		**out = **in
	}
	in.NodeBase.DeepCopyInto(&out.NodeBase)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPICSpec.
func (in *EPICSpec) DeepCopy() *EPICSpec {
	if in == nil {
		return nil
	}
	out := new(EPICSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPICStatus) DeepCopyInto(out *EPICStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPICStatus.
func (in *EPICStatus) DeepCopy() *EPICStatus {
	if in == nil {
		return nil
	}
	out := new(EPICStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Endpoint) DeepCopyInto(out *Endpoint) {
	*out = *in
	if in.Targets != nil {
		in, out := &in.Targets, &out.Targets
		*out = make(Targets, len(*in))
		copy(*out, *in)
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(Labels, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ProviderSpecific != nil {
		in, out := &in.ProviderSpecific, &out.ProviderSpecific
		*out = make(ProviderSpecific, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Endpoint.
func (in *Endpoint) DeepCopy() *Endpoint {
	if in == nil {
		return nil
	}
	out := new(Endpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GUETunnelEndpoint) DeepCopyInto(out *GUETunnelEndpoint) {
	*out = *in
	in.Port.DeepCopyInto(&out.Port)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GUETunnelEndpoint.
func (in *GUETunnelEndpoint) DeepCopy() *GUETunnelEndpoint {
	if in == nil {
		return nil
	}
	out := new(GUETunnelEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWCluster) DeepCopyInto(out *GWCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWCluster.
func (in *GWCluster) DeepCopy() *GWCluster {
	if in == nil {
		return nil
	}
	out := new(GWCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWClusterList) DeepCopyInto(out *GWClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GWCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWClusterList.
func (in *GWClusterList) DeepCopy() *GWClusterList {
	if in == nil {
		return nil
	}
	out := new(GWClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWClusterSpec) DeepCopyInto(out *GWClusterSpec) {
	*out = *in
	out.ClientRef = in.ClientRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWClusterSpec.
func (in *GWClusterSpec) DeepCopy() *GWClusterSpec {
	if in == nil {
		return nil
	}
	out := new(GWClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWClusterStatus) DeepCopyInto(out *GWClusterStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWClusterStatus.
func (in *GWClusterStatus) DeepCopy() *GWClusterStatus {
	if in == nil {
		return nil
	}
	out := new(GWClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWEndpointSlice) DeepCopyInto(out *GWEndpointSlice) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWEndpointSlice.
func (in *GWEndpointSlice) DeepCopy() *GWEndpointSlice {
	if in == nil {
		return nil
	}
	out := new(GWEndpointSlice)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWEndpointSlice) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWEndpointSliceList) DeepCopyInto(out *GWEndpointSliceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GWEndpointSlice, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWEndpointSliceList.
func (in *GWEndpointSliceList) DeepCopy() *GWEndpointSliceList {
	if in == nil {
		return nil
	}
	out := new(GWEndpointSliceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWEndpointSliceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWEndpointSliceSpec) DeepCopyInto(out *GWEndpointSliceSpec) {
	*out = *in
	out.ClientRef = in.ClientRef
	out.ParentRef = in.ParentRef
	in.EndpointSlice.DeepCopyInto(&out.EndpointSlice)
	if in.NodeAddresses != nil {
		in, out := &in.NodeAddresses, &out.NodeAddresses
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWEndpointSliceSpec.
func (in *GWEndpointSliceSpec) DeepCopy() *GWEndpointSliceSpec {
	if in == nil {
		return nil
	}
	out := new(GWEndpointSliceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWEndpointSliceStatus) DeepCopyInto(out *GWEndpointSliceStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWEndpointSliceStatus.
func (in *GWEndpointSliceStatus) DeepCopy() *GWEndpointSliceStatus {
	if in == nil {
		return nil
	}
	out := new(GWEndpointSliceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWProxy) DeepCopyInto(out *GWProxy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWProxy.
func (in *GWProxy) DeepCopy() *GWProxy {
	if in == nil {
		return nil
	}
	out := new(GWProxy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWProxy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWProxyList) DeepCopyInto(out *GWProxyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GWProxy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWProxyList.
func (in *GWProxyList) DeepCopy() *GWProxyList {
	if in == nil {
		return nil
	}
	out := new(GWProxyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWProxyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWProxySpec) DeepCopyInto(out *GWProxySpec) {
	*out = *in
	out.ClientRef = in.ClientRef
	if in.PublicPorts != nil {
		in, out := &in.PublicPorts, &out.PublicPorts
		*out = make([]corev1.ServicePort, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.EnvoyTemplate != nil {
		in, out := &in.EnvoyTemplate, &out.EnvoyTemplate
		*out = new(v1alpha1.EnvoyConfigSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.EnvoyReplicaCount != nil {
		in, out := &in.EnvoyReplicaCount, &out.EnvoyReplicaCount
		*out = new(int32)
		**out = **in
	}
	if in.GUETunnelMaps != nil {
		in, out := &in.GUETunnelMaps, &out.GUETunnelMaps
		*out = make(map[string]EPICEndpointMap, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.ProxyInterfaces != nil {
		in, out := &in.ProxyInterfaces, &out.ProxyInterfaces
		*out = make(map[string]ProxyInterfaceInfo, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.Endpoints != nil {
		in, out := &in.Endpoints, &out.Endpoints
		*out = make([]*Endpoint, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Endpoint)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWProxySpec.
func (in *GWProxySpec) DeepCopy() *GWProxySpec {
	if in == nil {
		return nil
	}
	out := new(GWProxySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWProxyStatus) DeepCopyInto(out *GWProxyStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWProxyStatus.
func (in *GWProxyStatus) DeepCopy() *GWProxyStatus {
	if in == nil {
		return nil
	}
	out := new(GWProxyStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWRoute) DeepCopyInto(out *GWRoute) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWRoute.
func (in *GWRoute) DeepCopy() *GWRoute {
	if in == nil {
		return nil
	}
	out := new(GWRoute)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWRoute) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWRouteList) DeepCopyInto(out *GWRouteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GWRoute, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWRouteList.
func (in *GWRouteList) DeepCopy() *GWRouteList {
	if in == nil {
		return nil
	}
	out := new(GWRouteList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GWRouteList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWRouteSpec) DeepCopyInto(out *GWRouteSpec) {
	*out = *in
	out.ClientRef = in.ClientRef
	in.HTTP.DeepCopyInto(&out.HTTP)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWRouteSpec.
func (in *GWRouteSpec) DeepCopy() *GWRouteSpec {
	if in == nil {
		return nil
	}
	out := new(GWRouteSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GWRouteStatus) DeepCopyInto(out *GWRouteStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GWRouteStatus.
func (in *GWRouteStatus) DeepCopy() *GWRouteStatus {
	if in == nil {
		return nil
	}
	out := new(GWRouteStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LBServiceGroup) DeepCopyInto(out *LBServiceGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LBServiceGroup.
func (in *LBServiceGroup) DeepCopy() *LBServiceGroup {
	if in == nil {
		return nil
	}
	out := new(LBServiceGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LBServiceGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LBServiceGroupList) DeepCopyInto(out *LBServiceGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]LBServiceGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LBServiceGroupList.
func (in *LBServiceGroupList) DeepCopy() *LBServiceGroupList {
	if in == nil {
		return nil
	}
	out := new(LBServiceGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LBServiceGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LBServiceGroupSpec) DeepCopyInto(out *LBServiceGroupSpec) {
	*out = *in
	if in.EnvoyImage != nil {
		in, out := &in.EnvoyImage, &out.EnvoyImage
		*out = new(string)
		**out = **in
	}
	in.EnvoyTemplate.DeepCopyInto(&out.EnvoyTemplate)
	in.EndpointTemplate.DeepCopyInto(&out.EndpointTemplate)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LBServiceGroupSpec.
func (in *LBServiceGroupSpec) DeepCopy() *LBServiceGroupSpec {
	if in == nil {
		return nil
	}
	out := new(LBServiceGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LBServiceGroupStatus) DeepCopyInto(out *LBServiceGroupStatus) {
	*out = *in
	if in.ProxySnapshotVersions != nil {
		in, out := &in.ProxySnapshotVersions, &out.ProxySnapshotVersions
		*out = make(map[string]int, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LBServiceGroupStatus.
func (in *LBServiceGroupStatus) DeepCopy() *LBServiceGroupStatus {
	if in == nil {
		return nil
	}
	out := new(LBServiceGroupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in Labels) DeepCopyInto(out *Labels) {
	{
		in := &in
		*out = make(Labels, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Labels.
func (in Labels) DeepCopy() Labels {
	if in == nil {
		return nil
	}
	out := new(Labels)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancer) DeepCopyInto(out *LoadBalancer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancer.
func (in *LoadBalancer) DeepCopy() *LoadBalancer {
	if in == nil {
		return nil
	}
	out := new(LoadBalancer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LoadBalancer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancerList) DeepCopyInto(out *LoadBalancerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]LoadBalancer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancerList.
func (in *LoadBalancerList) DeepCopy() *LoadBalancerList {
	if in == nil {
		return nil
	}
	out := new(LoadBalancerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LoadBalancerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancerSpec) DeepCopyInto(out *LoadBalancerSpec) {
	*out = *in
	if in.PublicPorts != nil {
		in, out := &in.PublicPorts, &out.PublicPorts
		*out = make([]corev1.ServicePort, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.EnvoyTemplate != nil {
		in, out := &in.EnvoyTemplate, &out.EnvoyTemplate
		*out = new(v1alpha1.EnvoyConfigSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.EnvoyReplicaCount != nil {
		in, out := &in.EnvoyReplicaCount, &out.EnvoyReplicaCount
		*out = new(int32)
		**out = **in
	}
	if in.UpstreamClusters != nil {
		in, out := &in.UpstreamClusters, &out.UpstreamClusters
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.GUETunnelMaps != nil {
		in, out := &in.GUETunnelMaps, &out.GUETunnelMaps
		*out = make(map[string]EPICEndpointMap, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.ProxyInterfaces != nil {
		in, out := &in.ProxyInterfaces, &out.ProxyInterfaces
		*out = make(map[string]ProxyInterfaceInfo, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.Endpoints != nil {
		in, out := &in.Endpoints, &out.Endpoints
		*out = make([]*Endpoint, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(Endpoint)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancerSpec.
func (in *LoadBalancerSpec) DeepCopy() *LoadBalancerSpec {
	if in == nil {
		return nil
	}
	out := new(LoadBalancerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancerStatus) DeepCopyInto(out *LoadBalancerStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancerStatus.
func (in *LoadBalancerStatus) DeepCopy() *LoadBalancerStatus {
	if in == nil {
		return nil
	}
	out := new(LoadBalancerStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Node) DeepCopyInto(out *Node) {
	*out = *in
	if in.IngressNICs != nil {
		in, out := &in.IngressNICs, &out.IngressNICs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.GUEEndpoint.DeepCopyInto(&out.GUEEndpoint)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Node.
func (in *Node) DeepCopy() *Node {
	if in == nil {
		return nil
	}
	out := new(Node)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in ProviderSpecific) DeepCopyInto(out *ProviderSpecific) {
	{
		in := &in
		*out = make(ProviderSpecific, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProviderSpecific.
func (in ProviderSpecific) DeepCopy() ProviderSpecific {
	if in == nil {
		return nil
	}
	out := new(ProviderSpecific)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProviderSpecificProperty) DeepCopyInto(out *ProviderSpecificProperty) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProviderSpecificProperty.
func (in *ProviderSpecificProperty) DeepCopy() *ProviderSpecificProperty {
	if in == nil {
		return nil
	}
	out := new(ProviderSpecificProperty)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxyInterfaceInfo) DeepCopyInto(out *ProxyInterfaceInfo) {
	*out = *in
	in.Port.DeepCopyInto(&out.Port)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxyInterfaceInfo.
func (in *ProxyInterfaceInfo) DeepCopy() *ProxyInterfaceInfo {
	if in == nil {
		return nil
	}
	out := new(ProxyInterfaceInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteEndpoint) DeepCopyInto(out *RemoteEndpoint) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteEndpoint.
func (in *RemoteEndpoint) DeepCopy() *RemoteEndpoint {
	if in == nil {
		return nil
	}
	out := new(RemoteEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteEndpoint) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteEndpointList) DeepCopyInto(out *RemoteEndpointList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RemoteEndpoint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteEndpointList.
func (in *RemoteEndpointList) DeepCopy() *RemoteEndpointList {
	if in == nil {
		return nil
	}
	out := new(RemoteEndpointList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteEndpointList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteEndpointSpec) DeepCopyInto(out *RemoteEndpointSpec) {
	*out = *in
	in.Port.DeepCopyInto(&out.Port)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteEndpointSpec.
func (in *RemoteEndpointSpec) DeepCopy() *RemoteEndpointSpec {
	if in == nil {
		return nil
	}
	out := new(RemoteEndpointSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteEndpointStatus) DeepCopyInto(out *RemoteEndpointStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteEndpointStatus.
func (in *RemoteEndpointStatus) DeepCopy() *RemoteEndpointStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteEndpointStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServicePrefix) DeepCopyInto(out *ServicePrefix) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServicePrefix.
func (in *ServicePrefix) DeepCopy() *ServicePrefix {
	if in == nil {
		return nil
	}
	out := new(ServicePrefix)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServicePrefix) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServicePrefixList) DeepCopyInto(out *ServicePrefixList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ServicePrefix, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServicePrefixList.
func (in *ServicePrefixList) DeepCopy() *ServicePrefixList {
	if in == nil {
		return nil
	}
	out := new(ServicePrefixList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServicePrefixList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServicePrefixSpec) DeepCopyInto(out *ServicePrefixSpec) {
	*out = *in
	if in.Gateway != nil {
		in, out := &in.Gateway, &out.Gateway
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServicePrefixSpec.
func (in *ServicePrefixSpec) DeepCopy() *ServicePrefixSpec {
	if in == nil {
		return nil
	}
	out := new(ServicePrefixSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServicePrefixStatus) DeepCopyInto(out *ServicePrefixStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServicePrefixStatus.
func (in *ServicePrefixStatus) DeepCopy() *ServicePrefixStatus {
	if in == nil {
		return nil
	}
	out := new(ServicePrefixStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in Targets) DeepCopyInto(out *Targets) {
	{
		in := &in
		*out = make(Targets, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Targets.
func (in Targets) DeepCopy() Targets {
	if in == nil {
		return nil
	}
	out := new(Targets)
	in.DeepCopyInto(out)
	return *out
}
