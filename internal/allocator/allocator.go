// Copyright 2017 Google Inc.
// Copyright 2020 Acnodal Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package allocator

import (
	"fmt"
	"net"

	epicv1 "epic-gateway.org/resource-model/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// An Allocator tracks IP address pools and allocates addresses from them.
type Allocator struct {
	pools     map[string]Pool
	allocated map[string]*alloc // svc -> alloc
}

type alloc struct {
	pool  string
	ip    net.IP
	ports []corev1.ServicePort
	Key
}

// NewAllocator returns an Allocator managing no pools.
func NewAllocator() *Allocator {
	return &Allocator{
		pools:     map[string]Pool{},
		allocated: map[string]*alloc{},
	}
}

// AddPrefix adds a ServicePrefix's address pool(s) to the
// allocator. Each SP will have at least one pool, but if that pool is
// IPV6 then it will also have an alternative pool of IPV4 addresses.
func (a *Allocator) AddPrefix(sp epicv1.ServicePrefix) error {
	pool, err := NewLocalPool(sp.Spec.PublicPool.Pool, sp.Spec.PublicPool.Subnet, sp.Spec.PublicPool.Aggregation)
	if err != nil {
		return fmt.Errorf("parsing address pool #%s: %s", sp.Name, err)
	}
	a.pools[sp.Name] = pool

	// Refresh or initiate stats
	poolCapacity.WithLabelValues(sp.Name).Set(float64(pool.Size()))
	poolActive.WithLabelValues(sp.Name).Set(float64(pool.InUse()))

	// Add this SP's alt pool (if it has one).
	if sp.Spec.AltPool != nil {
		fmt.Printf("adding alt pool %s\n", sp.Name+epicv1.AltAddressSuffix)
		pool, err := NewLocalPool(sp.Spec.AltPool.Pool, sp.Spec.AltPool.Subnet, sp.Spec.AltPool.Aggregation)
		if err != nil {
			return fmt.Errorf("parsing alt address pool #%s: %s", sp.Name, err)
		}
		a.pools[sp.Name+epicv1.AltAddressSuffix] = pool
	}

	return nil
}

// RemovePool removes an address pool from the allocator.
func (a *Allocator) RemovePool(sp epicv1.ServicePrefix) error {
	err := a.ValidateDelete(&sp)
	if err != nil {
		return err
	}
	delete(a.pools, sp.Name)

	poolCapacity.DeleteLabelValues(sp.Name)
	poolActive.DeleteLabelValues(sp.Name)

	return nil
}

// assign unconditionally updates internal state to reflect svc's
// allocation of alloc. Caller must ensure that this call is safe.
func (a *Allocator) assign(svc string, alloc *alloc) {
	a.Unassign(svc)
	a.allocated[svc] = alloc

	pool := a.pools[alloc.pool]
	pool.Assign(alloc.ip, alloc.ports, svc, &alloc.Key)

	poolCapacity.WithLabelValues(alloc.pool).Set(float64(a.pools[alloc.pool].Size()))
	poolActive.WithLabelValues(alloc.pool).Set(float64(pool.InUse()))
}

// Assign assigns the requested ip to svc, if the assignment is
// permissible by sharingKey.
func (a *Allocator) Assign(svc string, ip net.IP, ports []corev1.ServicePort, sharingKey string) (string, error) {
	pool := poolFor(a.pools, ip)
	if pool == "" {
		return "", fmt.Errorf("%q is not allowed in config", ip)
	}
	sk := &Key{
		Sharing: sharingKey,
	}

	// Does the IP already have allocs? If so, needs to be the same
	// sharing key, and have non-overlapping ports. If not, the proposed
	// IP needs to be allowed by configuration.
	err := a.pools[pool].Available(ip, ports, svc, sk) // FIXME: this should Assign() here, not check Available.  Might need to iterate over pools rather than do poolFor
	if err != nil {
		return "", err
	}

	// Either the IP is entirely unused, or the requested use is
	// compatible with existing uses. Assign!
	alloc := &alloc{
		pool:  pool,
		ip:    ip,
		ports: make([]corev1.ServicePort, len(ports)),
		Key:   *sk,
	}
	for i, port := range ports {
		alloc.ports[i] = port
	}
	a.assign(svc, alloc)
	return pool, nil
}

// Unassign frees the IP associated with service, if any.
func (a *Allocator) Unassign(svc string) bool {
	al := a.allocated[svc]
	if al == nil {
		// We don't know about the address but since the user asked us to
		// unassign it that's OK.
		return true
	}

	// tell the pool that the address has been released. there might not
	// be a pool, e.g., in the case of a config change that move
	// addresses from one pool to another
	pool, tracked := a.pools[al.pool]
	if tracked {
		pool.Release(al.ip, svc)
		poolActive.WithLabelValues(al.pool).Set(float64(pool.InUse()))
	}

	delete(a.allocated, svc)

	return true
}

// AllocateFromPool assigns an available IP from pool to service.
func (a *Allocator) AllocateFromPool(svc string, poolName string, ports []corev1.ServicePort, sharingKey string) (net.IP, error) {
	var ip net.IP

	// if we have already allocated an address for this service then
	// return it
	if alloc := a.allocated[svc]; alloc != nil {
		return alloc.ip, nil
	}

	pool := a.pools[poolName]
	if pool == nil {
		fmt.Printf("known pools: %#v\n", a.pools)
		return nil, fmt.Errorf("unknown pool %q", poolName)
	}

	sk := &Key{
		Sharing: sharingKey,
	}
	ip, err := pool.AssignNext(svc, ports, sk)
	if err != nil {
		// Woops, no IPs :( Fail.
		return nil, err
	}

	alloc := &alloc{
		pool:  poolName,
		ip:    ip,
		ports: make([]corev1.ServicePort, len(ports)),
		Key:   *sk,
	}
	for i, port := range ports {
		alloc.ports[i] = port
	}
	a.assign(svc, alloc)

	return ip, nil
}

// ip returns the ip address allocated to service, or nil if none are allocated.
func (a *Allocator) ip(svc string) net.IP {
	if alloc := a.allocated[svc]; alloc != nil {
		return alloc.ip
	}
	return nil
}

// ValidateDelete checks whether "sp" can be deleted.  A nil return
// value is good; non-nil means that this pool can't be deleted.
func (a *Allocator) ValidateDelete(sp *epicv1.ServicePrefix) error {
	// If the pool has any allocations then it can't be deleted.
	for _, alloc := range a.allocated {
		if alloc.pool == sp.Name {
			return fmt.Errorf("pool \"%s\" can't be deleted, addresses have been allocated from it", sp.Name)
		}
	}

	return nil
}

func poolFor(pools map[string]Pool, ip net.IP) string {
	for pname, p := range pools {
		if p.Contains(ip) {
			return pname
		}
	}
	return ""
}
