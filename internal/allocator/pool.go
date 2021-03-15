// Copyright 2017 Google Inc.
// Copyright 2020 Acnodal Inc.
//
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
	"errors"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
)

// Key represents a "sharing key" which is used to have two or more
// services share an IP address.
type Key struct {
	Sharing string
}

// Pool represents a pool of IP addresses.
type Pool interface {
	Available(net.IP, []corev1.ServicePort, string, *Key) error
	AssignNext(string, []corev1.ServicePort, *Key) (net.IP, error)
	Assign(net.IP, []corev1.ServicePort, string, *Key) error
	Release(net.IP, string)
	InUse() int
	SharingKey(net.IP) *Key
	Overlaps(Pool) bool
	Contains(net.IP) bool
	Size() uint64
}

func sharingOK(existing, new *Key) error {
	if existing.Sharing == "" {
		return errors.New("existing service does not allow sharing")
	}
	if new.Sharing == "" {
		return errors.New("new service does not allow sharing")
	}
	if existing.Sharing != new.Sharing {
		return fmt.Errorf("sharing key %q does not match existing sharing key %q", new.Sharing, existing.Sharing)
	}
	return nil
}
