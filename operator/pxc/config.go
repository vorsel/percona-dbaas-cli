// Copyright © 2019 Percona, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pxc

import (
	"fmt"

	"github.com/Percona-Lab/percona-dbaas-cli/operator/k8s"
	"github.com/Percona-Lab/percona-dbaas-cli/operator/pxc/types/config"
)

// PXDBCluster represent interface for ckuster types
type PXDBCluster interface {
	UpdateWith(c config.ClusterConfig, s3 *k8s.BackupStorageSpec) (err error)
	Upgrade(imgs map[string]string)
	SetNew(c config.ClusterConfig, s3 *k8s.BackupStorageSpec, p k8s.PlatformType) (err error)
	SetDefaults()
	GetName() string
	MarshalRequests() error
	GetCR() (string, error)
	SetLabels(labels map[string]string)
}

// Cluster is for package responses
type Cluster struct {
	Host  string           `json:"host,omitempty"`
	Port  int              `json:"port,omitempty"`
	User  string           `json:"user,omitempty"`
	Pass  string           `json:"pass,omitempty"`
	State k8s.ClusterState `json:"state,omitempty"`
}

func (c Cluster) String() string {
	stringMsg := `Host: %s, Port: 3306, User: root, Pass: %s`
	return fmt.Sprintf(stringMsg, c.Host, c.Pass)
}

type AppState string

const (
	AppStateUnknown AppState = "unknown"
	AppStateInit             = "initializing"
	AppStateReady            = "ready"
	AppStateError            = "error"
)

type PerconaXtraDBClusterStatus struct {
	PXC      AppStatus `json:"pxc,omitempty"`
	ProxySQL AppStatus `json:"proxysql,omitempty"`
	Host     string    `json:"host,omitempty"`
	Messages []string  `json:"message,omitempty"`
	Status   AppState  `json:"state,omitempty"`
}

type AppStatus struct {
	Size    int32    `json:"size,omitempty"`
	Ready   int32    `json:"ready,omitempty"`
	Status  AppState `json:"status,omitempty"`
	Message string   `json:"message,omitempty"`
}