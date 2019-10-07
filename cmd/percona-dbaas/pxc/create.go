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
	"regexp"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas"
	"github.com/Percona-Lab/percona-dbaas-cli/dbaas/pxc"
)

const (
	defaultVersion = "default"

	noS3backupWarn = `S3 backup storage options doesn't set: %v. You have specify S3 storage in order to make backups.
You can skip this step by using --s3-skip-storage flag add the storage later with the "add-storage" command.
`
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create-db <pxc-cluster-name>",
	Short: "Create MySQL cluster on current Kubernetes cluster",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("You have to specify pxc-cluster-name")
		}

		return nil
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		err := detectFormat(cmd)
		if err != nil {
			log.Error("detect output format:", err)
			return
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		dbservice, err := dbaas.New(*envCrt)
		if err != nil {
			log.Error("new dbservice:", err)
			return
		}
		if len(*labels) > 0 {
			match, err := regexp.MatchString("^(([a-zA-Z0-9_]+=[a-zA-Z0-9_]+)(,|$))+$", *labels)
			if err != nil {
				log.Error("label parse:", err)
				return
			}
			if !match {
				log.Error("Incorrect label format. Use key1=value1,key2=value2 syntax")
				return
			}
		}

		app := pxc.New(args[0], defaultVersion, *labels)

		config, err := pxc.ParseCreateFlagsToConfig(cmd.Flags())
		if err != nil {
			log.Error("parse flags to config:", err)
			return
		}

		var s3stor *dbaas.BackupStorageSpec
		if !*skipS3Storage {
			var err error
			s3stor, err = dbservice.S3Storage(app, config.S3)
			if err != nil {
				switch err.(type) {
				case dbaas.ErrNoS3Options:
					log.Errorf(noS3backupWarn, err)
				default:
					log.Error("create S3 backup storage:", err)
				}
				return
			}
		}

		setupmsg, err := app.Setup(config, s3stor, dbservice.GetPlatformType())
		if err != nil {
			log.Error("set configuration:", err)
			return
		}

		log.WithField("data", setupmsg).Info("Creating cluster")

		created := make(chan dbaas.Msg)
		msg := make(chan dbaas.OutuputMsg)
		cerr := make(chan error)

		go dbservice.Create("pxc", app, *operatorImage, created, msg, cerr)
		sp := spinner.New(spinner.CharSets[14], 250*time.Millisecond)
		sp.Color("green", "bold")
		demo, err := cmd.Flags().GetBool("demo")
		if demo && err == nil {
			sp.UpdateCharSet([]string{""})
		}
		sp.Lock()
		sp.Prefix = "Starting..."
		sp.Unlock()
		sp.Start()
		defer sp.Stop()
		for {
			select {
			case okmsg := <-created:
				sp.FinalMSG = ""
				sp.Stop()
				log.WithField("data", okmsg).Info("Cluster ready")
				return
			case omsg := <-msg:
				switch omsg.(type) {
				case dbaas.OutuputMsgDebug:
					// fmt.Printf("\n[debug] %s\n", omsg)
				case dbaas.OutuputMsgError:
					sp.Stop()
					log.Errorln("operator log error:", err.Error())
					sp.Start()
				}
			case err := <-cerr:
				sp.Stop()
				switch err.(type) {
				case dbaas.ErrAlreadyExists:
					log.Error("create pxc cluster:", err)
					list, err := dbservice.List("pxc")
					if err != nil {
						log.Error("list pxc clusters:", err)
						return
					}
					log.Println("Avaliable clusters:", list)
				default:
					log.Errorln("create pxc:", err.Error())
				}

				return
			}
		}
	},
}

var skipS3Storage *bool
var envCrt *string
var labels *string
var operatorImage *string

func init() {
	createCmd.Flags().String("storage-size", "6G", "PXC node volume size, in bytes (e,g. 5Gi = 5GiB = 5 * 1024 * 1024 * 1024)")
	createCmd.Flags().String("storage-class", "", "Name of the StorageClass required by the volume claim")
	createCmd.Flags().Int32("pxc-instances", 3, "Number of PXC nodes in cluster")
	createCmd.Flags().String("pxc-request-cpu", "600m", "PXC node requests for CPU, in cores. (500m = .5 cores)")
	createCmd.Flags().String("pxc-request-mem", "1G", "PXC node requests for memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)")
	createCmd.Flags().String("pxc-anti-affinity-key", "kubernetes.io/hostname", "Pod anti-affinity rules. Allowed values: none, kubernetes.io/hostname, failure-domain.beta.kubernetes.io/zone, failure-domain.beta.kubernetes.io/region")

	createCmd.Flags().Int32("proxy-instances", 1, "Number of ProxySQL nodes in cluster")
	createCmd.Flags().String("proxy-request-cpu", "600m", "ProxySQL node requests for CPU, in cores. (500m = .5 cores)")
	createCmd.Flags().String("proxy-request-mem", "1G", "ProxySQL node requests for memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)")
	createCmd.Flags().String("proxy-anti-affinity-key", "kubernetes.io/hostname", "Pod anti-affinity rules. Allowed values: none, kubernetes.io/hostname, failure-domain.beta.kubernetes.io/zone, failure-domain.beta.kubernetes.io/region")

	createCmd.Flags().String("s3-endpoint-url", "", "Endpoing URL of S3 compatible storage to store backup at")
	createCmd.Flags().String("s3-bucket", "", "Bucket of S3 compatible storage to store backup at")
	createCmd.Flags().String("s3-region", "", "Region of S3 compatible storage to store backup at")
	createCmd.Flags().String("s3-credentials-secret", "", "Secrets with credentials for S3 compatible storage to store backup at. Alternatevily you can set --s3-access-key-id and --s3-secret-access-key instead.")
	createCmd.Flags().String("s3-access-key-id", "", "Access Key ID for S3 compatible storage to store backup at")
	createCmd.Flags().String("s3-secret-access-key", "", "Access Key for S3 compatible storage to store backup at")
	createCmd.Flags().Bool("pmm-enabled", false, "Activate monitoring service.")
	createCmd.Flags().String("pmm-image", "perconalab/pmm-client:1.17.1", "Monitoring service client image.")
	createCmd.Flags().String("pmm-server-host", "monitoring-service", "Monitoring service server hostname.")
	createCmd.Flags().String("pmm-server-user", "", "Monitoring service user for login.")
	createCmd.Flags().String("pmm-server-password", "", "Monitoring service password for login.")
	skipS3Storage = createCmd.Flags().Bool("s3-skip-storage", false, "Don't create S3 compatible backup storage. Has to be set manually later on.")
	envCrt = createCmd.Flags().String("environment", "", "Target kubernetes cluster")
	labels = createCmd.Flags().String("labels", "", "PXC cluster labels inside kubernetes/openshift cluster")
	operatorImage = createCmd.Flags().String("operator-image", "", "Custom operator image")

	PXCCmd.AddCommand(createCmd)
}
