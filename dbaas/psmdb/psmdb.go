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

package psmdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas"
)

type Version string

const (
	CurrentVersion Version = "1.1.0"

	defaultRSname          = "rs0"
	defaultOperatorVersion = "perconalab/percona-server-mongodb-operator:1.1.0"
)

type PSMDB struct {
	name         string
	rsName       string
	config       *PerconaServerMongoDB
	obj          dbaas.Objects
	dbpass       []byte
	opLogsLastTS float64
}

func New(clusterName, replsetName string, version Version) *PSMDB {
	if replsetName == "" {
		replsetName = defaultRSname
	}

	return &PSMDB{
		name:   clusterName,
		rsName: replsetName,
		obj:    objects[version],
		config: &PerconaServerMongoDB{},
	}
}

func (p PSMDB) Bundle(operatorVersion string) []dbaas.BundleObject {
	if operatorVersion == "" {
		operatorVersion = defaultOperatorVersion
	}

	for i, o := range p.obj.Bundle {
		if o.Kind == "Deployment" && o.Name == p.OperatorName() {
			p.obj.Bundle[i].Data = strings.Replace(o.Data, "{{image}}", operatorVersion, -1)
		}
	}
	return p.obj.Bundle
}

func (p PSMDB) Name() string {
	return p.name
}

func (p PSMDB) App() (string, error) {
	cr, err := json.Marshal(p.config)
	if err != nil {
		return "", errors.Wrap(err, "marshal cr template")
	}

	return string(cr), nil
}

const createMsg = `Create MongoDB cluster.
 
Replica Set Name        | %v
Replica Set Size        | %v
Storage                 | %v
`

func (p *PSMDB) Setup(f *pflag.FlagSet, s3 *dbaas.BackupStorageSpec) (string, error) {
	err := p.config.SetNew(p.Name(), p.rsName, f, s3, dbaas.GetPlatformType())

	if err != nil {
		return "", errors.Wrap(err, "parse options")
	}

	storage, err := p.config.Spec.Replsets[0].VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage].MarshalJSON()
	if err != nil {
		return "", errors.Wrap(err, "marshal psmdb volume requests")
	}

	return fmt.Sprintf(createMsg, p.config.Spec.Replsets[0].Name, p.config.Spec.Replsets[0].Size, string(storage)), nil
}

const updateMsg = `Update MongoDB cluster.
 
Replica Set Name        | %v
Replica Set Size        | %v
`

func (p *PSMDB) Edit(crRaw []byte, f *pflag.FlagSet, storage *dbaas.BackupStorageSpec) (string, error) {
	cr := &PerconaServerMongoDB{}
	err := json.Unmarshal(crRaw, cr)
	if err != nil {
		return "", errors.Wrap(err, "unmarshal current cr")
	}

	p.config.APIVersion = cr.APIVersion
	p.config.Kind = cr.Kind
	p.config.Name = cr.Name
	p.config.Spec = cr.Spec
	p.config.Status = cr.Status

	err = p.config.UpdateWith(p.rsName, f, storage)
	if err != nil {
		return "", errors.Wrap(err, "apply changes to cr")
	}

	return fmt.Sprintf(updateMsg, p.config.Spec.Replsets[0].Name, p.config.Spec.Replsets[0].Size), nil
}

func (p *PSMDB) Upgrade(crRaw []byte, newImages map[string]string) error {
	cr := &PerconaServerMongoDB{}
	err := json.Unmarshal(crRaw, cr)
	if err != nil {
		return errors.Wrap(err, "unmarshal current cr")
	}

	p.config.APIVersion = cr.APIVersion
	p.config.Kind = cr.Kind
	p.config.Name = cr.Name
	p.config.Spec = cr.Spec
	p.config.Status = cr.Status

	p.config.Upgrade(newImages)

	return nil
}

const operatorImage = "percona/percona-server-mongodb-operator:"

func (p *PSMDB) Images(ver string, f *pflag.FlagSet) (operator string, apps map[string]string, err error) {
	apps = make(map[string]string)
	if ver != "" {
		operator = operatorImage + ver
		apps["psmdb"] = operatorImage + ver + "-mongod4.0"
		apps["backup"] = operatorImage + ver + "-backup"
	}

	op, err := f.GetString("operator-image")
	if err != nil {
		return operator, apps, errors.New("undefined `operator-image`")
	}
	if op != "" {
		operator = op
	}

	psmdb, err := f.GetString("psmdb-image")
	if err != nil {
		return operator, apps, errors.New("undefined `psmdb-image`")
	}
	if psmdb != "" {
		apps["psmdb"] = psmdb
	}

	backup, err := f.GetString("backup-image")
	if err != nil {
		return operator, apps, errors.New("undefined `backup-image`")
	}
	if backup != "" {
		apps["backup"] = backup
	}

	return operator, apps, nil
}

func (p *PSMDB) OperatorName() string {
	return "percona-server-mongodb-operator"
}

type k8sStatus struct {
	Status PerconaServerMongoDBStatus
}

const okmsg = `
MongoDB cluster started successfully, right endpoint for application:
Host: %s
Port: 27017
ClusterAdmin User: %s
ClusterAdmin Password: %s
UserAdmin User: %s
UserAdmin Password: %s

Enjoy!`

func (p *PSMDB) CheckStatus(data []byte, pass map[string][]byte) (dbaas.ClusterState, []string, error) {
	st := &k8sStatus{}

	err := json.Unmarshal(data, st)
	if err != nil {
		return dbaas.ClusterStateUnknown, nil, errors.Wrap(err, "unmarshal status")
	}

	status := st.Status.Replsets[p.rsName]
	if status == nil {
		switch st.Status.Status {
		case AppStateReady:
			host := fmt.Sprintf("%[1]s-%[2]s-0.%[1]s-%[2]s", p.name, p.rsName)
			msg := fmt.Sprintf(okmsg, host, pass["MONGODB_CLUSTER_ADMIN_USER"], pass["MONGODB_CLUSTER_MONITOR_PASSWORD"], pass["MONGODB_USER_ADMIN_USER"], pass["MONGODB_USER_ADMIN_PASSWORD"])
			return dbaas.ClusterStateReady, []string{msg}, nil
		case AppStateError:
			return dbaas.ClusterStateError, alterStatusMgs([]string{status.Message}), nil
		default:
			return dbaas.ClusterStateInit, nil, nil
		}
	}

	switch status.Status {
	case AppStateReady:
		host := fmt.Sprintf("%[1]s-%[2]s-0.%[1]s-%[2]s", p.name, p.rsName)
		msg := fmt.Sprintf(okmsg, host, pass["MONGODB_CLUSTER_ADMIN_USER"], pass["MONGODB_CLUSTER_MONITOR_PASSWORD"], pass["MONGODB_USER_ADMIN_USER"], pass["MONGODB_USER_ADMIN_PASSWORD"])
		return dbaas.ClusterStateReady, []string{msg}, nil
	case AppStateError:
		return dbaas.ClusterStateError, alterStatusMgs([]string{status.Message}), nil
	default:
		return dbaas.ClusterStateInit, nil, nil
	}

	return dbaas.ClusterStateInit, nil, nil
}

type operatorLog struct {
	Level      string  `json:"level"`
	TS         float64 `json:"ts"`
	Msg        string  `json:"msg"`
	Error      string  `json:"error"`
	Request    string  `json:"request"`
	Controller string  `json:"controller"`
}

func (p *PSMDB) CheckOperatorLogs(data []byte) ([]dbaas.OutuputMsg, error) {
	msgs := []dbaas.OutuputMsg{}

	lines := bytes.Split(data, []byte("\n"))
	for _, l := range lines {
		if len(l) == 0 {
			continue
		}

		entry := &operatorLog{}
		err := json.Unmarshal(l, entry)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal entry")
		}

		if entry.Controller != "psmdb-controller" {
			continue
		}

		// skips old entries
		if entry.TS <= p.opLogsLastTS {
			continue
		}

		p.opLogsLastTS = entry.TS

		if entry.Level != "error" {
			continue
		}

		cluster := ""
		s := strings.Split(entry.Request, "/")
		if len(s) == 2 {
			cluster = s[1]
		}

		if cluster != p.name {
			continue
		}

		msgs = append(msgs, alterOpError(entry))
	}

	return msgs, nil
}

func alterOpError(l *operatorLog) dbaas.OutuputMsg {
	if strings.Contains(l.Error, "the object has been modified; please apply your changes to the latest version and try again") {
		if i := strings.Index(l.Error, "Operation cannot be fulfilled on"); i >= 0 {
			return dbaas.OutuputMsgDebug(l.Error[i:])
		}
	}

	return dbaas.OutuputMsgError(l.Msg + ": " + l.Error)
}

func alterStatusMgs(msgs []string) []string {
	for i, msg := range msgs {
		msgs[i] = alterMessage(msg)
	}

	return msgs
}

func alterMessage(msg string) string {
	if strings.Contains(msg, "node(s) didn't match pod affinity/anti-affinity") {
		return "Cluster node(s) didn't satisfy pods [anti-]affinity rules. Try to change --anti-affinity-key parameter or add more nodes/change topology of your cluster."
	}

	if strings.Contains(msg, "Insufficient memory.") {
		return "Avaliable memory not enough to satisfy replica set request. Try to change --request-mem parameter or add more memmory to your cluster."
	}

	if strings.Contains(msg, "Insufficient cpu.") {
		return "Avaliable CPU not enough to satisfy replica set request. Try to change --request-cpu parameter or add more CPU to your cluster."
	}

	return msg
}